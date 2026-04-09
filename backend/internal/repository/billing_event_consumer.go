package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	group          string
	consumer       string
	batchSize      int64
	blockTimeout   time.Duration
	pendingRecover time.Duration
	workers        int

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewRedisBillingEventConsumer creates a consumer that reads billing events from a Redis Stream.
func NewRedisBillingEventConsumer(rdb *redis.Client, cfg *config.Config) service.BillingEventConsumer {
	streamCfg := cfg.Billing.Stream
	key := streamCfg.Key
	if key == "" {
		key = "billing:events"
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
		group:          group,
		consumer:       hostname,
		batchSize:      int64(batchSize),
		blockTimeout:   blockTimeout,
		pendingRecover: pendingRecover,
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
				c.processMessage(ctx, handler, msg, consumerName)
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
	pending, err := c.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: c.key,
		Group:  c.group,
		Start:  "-",
		End:    "+",
		Count:  c.batchSize,
		Idle:   c.pendingRecover,
	}).Result()
	if err != nil {
		if ctx.Err() == nil {
			logger.L().Warn("billing consumer: pending check failed",
				zap.String("component", "repository.billing_event_consumer"),
				zap.Error(err),
			)
		}
		return
	}
	if len(pending) == 0 {
		return
	}

	ids := make([]string, 0, len(pending))
	for _, p := range pending {
		ids = append(ids, p.ID)
	}

	claimed, err := c.rdb.XClaim(ctx, &redis.XClaimArgs{
		Stream:   c.key,
		Group:    c.group,
		Consumer: c.consumer + "-recovery",
		MinIdle:  c.pendingRecover,
		Messages: ids,
	}).Result()
	if err != nil {
		if ctx.Err() == nil {
			logger.L().Warn("billing consumer: claim failed",
				zap.String("component", "repository.billing_event_consumer"),
				zap.Error(err),
			)
		}
		return
	}

	for _, msg := range claimed {
		c.processMessage(ctx, handler, msg, c.consumer+"-recovery")
	}
}

// processMessage deserializes and processes a single stream message, then ACKs it.
func (c *redisBillingEventConsumer) processMessage(ctx context.Context, handler service.BillingEventConsumerHandler, msg redis.XMessage, consumerName string) {
	dataStr, ok := msg.Values["data"].(string)
	if !ok {
		logger.L().Error("billing consumer: missing data field",
			zap.String("component", "repository.billing_event_consumer"),
			zap.String("msg_id", msg.ID),
		)
		// ACK to avoid reprocessing bad messages forever.
		_ = c.rdb.XAck(ctx, c.key, c.group, msg.ID).Err()
		return
	}

	var event service.BillingEvent
	if err := json.Unmarshal([]byte(dataStr), &event); err != nil {
		logger.L().Error("billing consumer: unmarshal failed",
			zap.String("component", "repository.billing_event_consumer"),
			zap.String("msg_id", msg.ID),
			zap.Error(err),
		)
		// ACK to avoid reprocessing permanently corrupt messages.
		_ = c.rdb.XAck(ctx, c.key, c.group, msg.ID).Err()
		return
	}

	if err := handler(ctx, &event); err != nil {
		logger.L().Error("billing consumer: handler failed",
			zap.String("component", "repository.billing_event_consumer"),
			zap.String("msg_id", msg.ID),
			zap.String("consumer", consumerName),
			zap.Error(err),
		)
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
