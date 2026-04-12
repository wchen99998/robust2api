package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	appelotel "github.com/Wei-Shaw/sub2api/internal/pkg/otel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// redisBillingEventPublisher publishes usage charge events to a Redis Stream.
type redisBillingEventPublisher struct {
	rdb            *redis.Client
	key            string
	maxLen         int64
	retries        int
	publishTimeout time.Duration
}

// NewRedisBillingEventPublisher creates a publisher that writes billing events to a Redis Stream.
func NewRedisBillingEventPublisher(rdb *redis.Client, cfg *config.Config) service.BillingEventPublisher {
	streamCfg := cfg.Billing.Stream
	key := streamCfg.Key
	if key == "" {
		key = "billing:events"
	}
	maxLen := streamCfg.MaxLen
	retries := streamCfg.PublishRetries
	if retries <= 0 {
		retries = 3
	}
	publishTimeout := time.Duration(streamCfg.PublishTimeoutSeconds) * time.Second
	if publishTimeout <= 0 {
		publishTimeout = 10 * time.Second
	}
	return &redisBillingEventPublisher{
		rdb:            rdb,
		key:            key,
		maxLen:         maxLen,
		retries:        retries,
		publishTimeout: publishTimeout,
	}
}

// Publish serializes the event to JSON and appends it to the billing Redis Stream.
// On failure, it retries with exponential backoff up to the configured retry count.
func (p *redisBillingEventPublisher) Publish(ctx context.Context, event *service.UsageChargeEvent) error {
	if event == nil {
		return nil
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("billing event marshal: %w", err)
	}

	var lastErr error
	startedAt := time.Now()
	for attempt := 0; attempt < p.retries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				appelotel.M().RecordBillingPublish(ctx, "canceled", time.Since(startedAt).Seconds())
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
		args := &redis.XAddArgs{
			Stream: p.key,
			Values: map[string]interface{}{"data": string(data)},
		}
		if p.maxLen > 0 {
			// Financial events are never trimmed by default. If retention is explicitly
			// configured, use exact trimming so operators can reason about the bound.
			args.MaxLen = p.maxLen
			args.Approx = false
		}
		attemptCtx := ctx
		cancel := func() {}
		if p.publishTimeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, p.publishTimeout)
		}
		lastErr = p.rdb.XAdd(attemptCtx, args).Err()
		cancel()
		if lastErr == nil {
			appelotel.M().RecordBillingPublish(ctx, "success", time.Since(startedAt).Seconds())
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			appelotel.M().RecordBillingPublish(ctx, "canceled", time.Since(startedAt).Seconds())
			return ctxErr
		}
		logger.L().Warn("billing event publish retry",
			zap.String("component", "repository.billing_event_stream"),
			zap.Int("attempt", attempt+1),
			zap.Error(lastErr),
		)
	}
	appelotel.M().RecordBillingPublish(ctx, "failure", time.Since(startedAt).Seconds())
	return fmt.Errorf("billing event publish failed after %d attempts: %w", p.retries, lastErr)
}
