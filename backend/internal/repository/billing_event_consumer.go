package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// redisBillingEventConsumer reads BillingEvents from a Redis Stream using consumer groups.
type redisBillingEventConsumer struct {
	rdb            *redis.Client
	key            string
	dlqKey         string
	dlqMaxLen      int64
	group          string
	consumer       string
	batchSize      int64
	blockTimeout   time.Duration
	pendingRecover time.Duration
	maxRetryCount  int64
	workers        int

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

const billingStatusPendingScanBatchSize int64 = 1000

// NewRedisBillingEventConsumer creates a consumer that reads billing events from a Redis Stream.
func NewRedisBillingEventConsumer(rdb *redis.Client, cfg *config.Config) service.BillingEventConsumer {
	streamCfg := cfg.Billing.Stream
	key := streamCfg.Key
	if key == "" {
		key = "billing:events"
	}
	dlqKey := streamCfg.DLQKey
	if dlqKey == "" {
		dlqKey = key + ":dlq"
	}
	dlqMaxLen := streamCfg.DLQMaxLen
	if dlqMaxLen <= 0 {
		dlqMaxLen = 100000
	}
	group := streamCfg.ConsumerGroup
	if group == "" {
		group = "billing-workers"
	}
	batchSize := streamCfg.BatchSize
	if batchSize <= 0 {
		batchSize = 50
	}
	blockTimeout := time.Duration(streamCfg.BlockTimeoutSeconds) * time.Second
	if blockTimeout <= 0 {
		blockTimeout = 5 * time.Second
	}
	pendingRecover := time.Duration(streamCfg.PendingRecoverySeconds) * time.Second
	if pendingRecover <= 0 {
		pendingRecover = 30 * time.Second
	}
	maxRetryCount := int64(streamCfg.MaxRetryCount)
	if maxRetryCount <= 0 {
		maxRetryCount = 20
	}
	workers := streamCfg.Workers
	if workers <= 0 {
		workers = 4
	}
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "billing"
	}

	return &redisBillingEventConsumer{
		rdb:            rdb,
		key:            key,
		dlqKey:         dlqKey,
		dlqMaxLen:      dlqMaxLen,
		group:          group,
		consumer:       hostname,
		batchSize:      int64(batchSize),
		blockTimeout:   blockTimeout,
		pendingRecover: pendingRecover,
		maxRetryCount:  maxRetryCount,
		workers:        workers,
	}
}

// Start begins consuming billing events in background goroutines.
// It creates the consumer group if it doesn't exist and spawns worker goroutines.
func (c *redisBillingEventConsumer) Start(handler service.BillingEventConsumerHandler) {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel

	// Ensure consumer group exists.
	if err := c.rdb.XGroupCreateMkStream(ctx, c.key, c.group, "0").Err(); err != nil {
		// BUSYGROUP = group already exists, safe to ignore.
		if !redis.HasErrorPrefix(err, "BUSYGROUP") {
			logger.L().Error("billing consumer: create group failed",
				zap.String("component", "repository.billing_event_consumer"),
				zap.Error(err),
			)
		}
	}

	// Start worker goroutines for new messages.
	for i := 0; i < c.workers; i++ {
		c.wg.Add(1)
		go c.readLoop(ctx, handler, fmt.Sprintf("%s-%d", c.consumer, i))
	}

	// Start a single goroutine for pending message recovery.
	c.wg.Add(1)
	go c.pendingRecoveryLoop(ctx, handler)

	logger.L().Info("billing consumer started",
		zap.String("component", "repository.billing_event_consumer"),
		zap.String("stream", c.key),
		zap.String("group", c.group),
		zap.Int("workers", c.workers),
	)
}

// Stop gracefully shuts down all consumer goroutines and waits for them to finish.
func (c *redisBillingEventConsumer) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	logger.L().Info("billing consumer stopped",
		zap.String("component", "repository.billing_event_consumer"),
	)
}

func (c *redisBillingEventConsumer) Status(ctx context.Context) (*service.BillingConsumerStatus, error) {
	status := &service.BillingConsumerStatus{
		StreamKey: c.key,
		Group:     c.group,
		DLQKey:    c.dlqKey,
	}
	pending, err := c.rdb.XPending(ctx, c.key, c.group).Result()
	if err != nil {
		return nil, err
	}
	status.PendingCount = pending.Count
	if pending.Count > 0 {
		oldestPendingAge, err := scanOldestPendingAge(pending.Count, billingStatusPendingScanBatchSize, func(start string, count int64) ([]redis.XPendingExt, error) {
			return c.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
				Stream: c.key,
				Group:  c.group,
				Start:  start,
				End:    "+",
				Count:  count,
			}).Result()
		})
		if err != nil {
			return nil, err
		}
		status.OldestPendingAge = oldestPendingAge
	}
	if c.dlqKey != "" {
		dlqDepth, err := c.rdb.XLen(ctx, c.dlqKey).Result()
		if err != nil {
			return nil, err
		}
		status.DLQDepth = dlqDepth
	}
	return status, nil
}

func scanOldestPendingAge(pendingCount int64, pageSize int64, fetch func(start string, count int64) ([]redis.XPendingExt, error)) (time.Duration, error) {
	if pendingCount <= 0 || fetch == nil {
		return 0, nil
	}
	if pageSize <= 0 {
		pageSize = billingStatusPendingScanBatchSize
	}

	start := "-"
	scanned := int64(0)
	var oldest time.Duration

	for scanned < pendingCount {
		count := pageSize
		if remaining := pendingCount - scanned; remaining < count {
			count = remaining
		}
		entries, err := fetch(start, count)
		if err != nil {
			return 0, err
		}
		if len(entries) == 0 {
			break
		}
		for _, entry := range entries {
			if entry.Idle > oldest {
				oldest = entry.Idle
			}
		}
		scanned += int64(len(entries))
		if scanned >= pendingCount {
			break
		}
		nextStart := nextPendingScanStart(entries[len(entries)-1].ID)
		if nextStart == "" || nextStart == start {
			break
		}
		start = nextStart
	}

	return oldest, nil
}

func nextPendingScanStart(id string) string {
	msStr, seqStr, ok := strings.Cut(strings.TrimSpace(id), "-")
	if !ok {
		return ""
	}
	ms, err := strconv.ParseUint(msStr, 10, 64)
	if err != nil {
		return ""
	}
	seq, err := strconv.ParseUint(seqStr, 10, 64)
	if err != nil {
		return ""
	}
	if seq == math.MaxUint64 {
		if ms == math.MaxUint64 {
			return ""
		}
		return fmt.Sprintf("%d-0", ms+1)
	}
	return fmt.Sprintf("%d-%d", ms, seq+1)
}

func scanPendingEntries(start string, pageSize int64, fetch func(start string, count int64) ([]redis.XPendingExt, error), handle func([]redis.XPendingExt) error) error {
	if fetch == nil || handle == nil {
		return nil
	}
	if pageSize <= 0 {
		pageSize = 1
	}
	if strings.TrimSpace(start) == "" {
		start = "-"
	}
	for {
		entries, err := fetch(start, pageSize)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			return nil
		}
		if err := handle(entries); err != nil {
			return err
		}
		nextStart := nextPendingScanStart(entries[len(entries)-1].ID)
		if nextStart == "" || nextStart == start {
			return nil
		}
		start = nextStart
	}
}

func pendingRetryCountsByID(entries []redis.XPendingExt) map[string]int64 {
	retryCounts := make(map[string]int64, len(entries))
	for _, entry := range entries {
		retryCounts[entry.ID] = entry.RetryCount
	}
	return retryCounts
}

func pendingIDs(entries []redis.XPendingExt) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	return ids
}

// readLoop reads new messages from the stream using XREADGROUP with ">".
func (c *redisBillingEventConsumer) readLoop(ctx context.Context, handler service.BillingEventConsumerHandler, consumerName string) {
	defer c.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Consumer: consumerName,
			Streams:  []string{c.key, ">"},
			Count:    c.batchSize,
			Block:    c.blockTimeout,
		}).Result()
		if err != nil {
			if err == redis.Nil || ctx.Err() != nil {
				continue
			}
			logger.L().Warn("billing consumer: read error",
				zap.String("component", "repository.billing_event_consumer"),
				zap.String("consumer", consumerName),
				zap.Error(err),
			)
			time.Sleep(time.Second)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				c.processMessage(ctx, handler, msg, consumerName, 0)
			}
		}
	}
}

// pendingRecoveryLoop periodically claims and reprocesses pending messages
// that were not acknowledged (e.g., due to consumer crash).
func (c *redisBillingEventConsumer) pendingRecoveryLoop(ctx context.Context, handler service.BillingEventConsumerHandler) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.pendingRecover)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.recoverPending(ctx, handler)
		}
	}
}

// recoverPending claims messages that have been pending for longer than the recovery interval.
func (c *redisBillingEventConsumer) recoverPending(ctx context.Context, handler service.BillingEventConsumerHandler) {
	consumerName := c.consumer + "-recovery"
	err := scanPendingEntries("-", c.batchSize, func(start string, count int64) ([]redis.XPendingExt, error) {
		return c.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
			Stream: c.key,
			Group:  c.group,
			Start:  start,
			End:    "+",
			Count:  count,
			Idle:   c.pendingRecover,
		}).Result()
	}, func(entries []redis.XPendingExt) error {
		claimed, err := c.rdb.XClaim(ctx, &redis.XClaimArgs{
			Stream:   c.key,
			Group:    c.group,
			Consumer: consumerName,
			MinIdle:  c.pendingRecover,
			Messages: pendingIDs(entries),
		}).Result()
		if err != nil {
			return err
		}
		retryCountByID := pendingRetryCountsByID(entries)
		for _, msg := range claimed {
			c.processMessage(ctx, handler, msg, consumerName, retryCountByID[msg.ID])
		}
		return nil
	})
	if err != nil && ctx.Err() == nil {
		logger.L().Warn("billing consumer: recover pending failed",
			zap.String("component", "repository.billing_event_consumer"),
			zap.Error(err),
		)
	}
}

// processMessage deserializes and processes a single stream message, then ACKs it.
func (c *redisBillingEventConsumer) processMessage(ctx context.Context, handler service.BillingEventConsumerHandler, msg redis.XMessage, consumerName string, retryCount int64) {
	dataStr, ok := msg.Values["data"].(string)
	if !ok {
		logger.L().Error("billing consumer: missing data field",
			zap.String("component", "repository.billing_event_consumer"),
			zap.String("msg_id", msg.ID),
		)
		c.deadLetterMessage(ctx, msg, consumerName, retryCount, "missing_data_field", "")
		_ = c.rdb.XAck(ctx, c.key, c.group, msg.ID).Err()
		return
	}

	var event service.UsageChargeEvent
	if err := json.Unmarshal([]byte(dataStr), &event); err != nil {
		logger.L().Error("billing consumer: unmarshal failed",
			zap.String("component", "repository.billing_event_consumer"),
			zap.String("msg_id", msg.ID),
			zap.Error(err),
		)
		c.deadLetterMessage(ctx, msg, consumerName, retryCount, "unmarshal_failed", dataStr)
		_ = c.rdb.XAck(ctx, c.key, c.group, msg.ID).Err()
		return
	}

	if retryCount >= c.maxRetryCount && c.maxRetryCount > 0 {
		logger.L().Error("billing consumer: max retries exceeded",
			zap.String("component", "repository.billing_event_consumer"),
			zap.String("msg_id", msg.ID),
			zap.String("consumer", consumerName),
			zap.Int64("retry_count", retryCount),
		)
		c.deadLetterMessage(ctx, msg, consumerName, retryCount, "max_retries_exceeded", dataStr)
		_ = c.rdb.XAck(ctx, c.key, c.group, msg.ID).Err()
		return
	}

	if err := handler(ctx, &event); err != nil {
		logger.L().Error("billing consumer: handler failed",
			zap.String("component", "repository.billing_event_consumer"),
			zap.String("msg_id", msg.ID),
			zap.String("consumer", consumerName),
			zap.Int64("retry_count", retryCount),
			zap.Error(err),
		)
		if retryCount >= c.maxRetryCount && c.maxRetryCount > 0 {
			c.deadLetterMessage(ctx, msg, consumerName, retryCount, "handler_failed", dataStr)
			_ = c.rdb.XAck(ctx, c.key, c.group, msg.ID).Err()
		}
		// Do NOT ACK — the message stays pending for retry via pendingRecoveryLoop.
		return
	}

	if err := c.rdb.XAck(ctx, c.key, c.group, msg.ID).Err(); err != nil {
		logger.L().Warn("billing consumer: ack failed",
			zap.String("component", "repository.billing_event_consumer"),
			zap.String("msg_id", msg.ID),
			zap.Error(err),
		)
	}
}

func (c *redisBillingEventConsumer) deadLetterMessage(ctx context.Context, msg redis.XMessage, consumerName string, retryCount int64, reason string, data string) {
	if c.dlqKey == "" {
		return
	}
	if data == "" {
		if raw, ok := msg.Values["data"].(string); ok {
			data = raw
		} else {
			data = fmt.Sprintf("%v", msg.Values)
		}
	}

	if err := c.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: c.dlqKey,
		MaxLen: c.dlqMaxLen,
		Approx: true,
		Values: map[string]interface{}{
			"data":              data,
			"source_stream":     c.key,
			"source_group":      c.group,
			"source_message_id": msg.ID,
			"consumer":          consumerName,
			"retry_count":       retryCount,
			"reason":            reason,
			"failed_at":         time.Now().UTC().Format(time.RFC3339Nano),
		},
	}).Err(); err != nil {
		logger.L().Warn("billing consumer: dlq publish failed",
			zap.String("component", "repository.billing_event_consumer"),
			zap.String("msg_id", msg.ID),
			zap.String("dlq_stream", c.dlqKey),
			zap.Error(err),
		)
	}
}
