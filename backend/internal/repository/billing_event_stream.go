package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// redisBillingEventPublisher publishes BillingEvents to a Redis Stream.
type redisBillingEventPublisher struct {
	rdb     *redis.Client
	key     string
	maxLen  int64
	retries int
}

// NewRedisBillingEventPublisher creates a publisher that writes billing events to a Redis Stream.
func NewRedisBillingEventPublisher(rdb *redis.Client, cfg *config.Config) service.BillingEventPublisher {
	streamCfg := cfg.Billing.Stream
	key := streamCfg.Key
	if key == "" {
		key = "billing:events"
	}
	maxLen := streamCfg.MaxLen
	if maxLen <= 0 {
		maxLen = 100000
	}
	retries := streamCfg.PublishRetries
	if retries <= 0 {
		retries = 3
	}
	return &redisBillingEventPublisher{
		rdb:     rdb,
		key:     key,
		maxLen:  maxLen,
		retries: retries,
	}
}

// Publish serializes the event to JSON and appends it to the billing Redis Stream.
// On failure, it retries with exponential backoff up to the configured retry count.
func (p *redisBillingEventPublisher) Publish(ctx context.Context, event *service.BillingEvent) error {
	if event == nil {
		return nil
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("billing event marshal: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < p.retries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
		lastErr = p.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: p.key,
			MaxLen: p.maxLen,
			Approx: true,
			Values: map[string]interface{}{"data": string(data)},
		}).Err()
		if lastErr == nil {
			return nil
		}
		logger.L().Warn("billing event publish retry",
			zap.String("component", "repository.billing_event_stream"),
			zap.Int("attempt", attempt+1),
			zap.Error(lastErr),
		)
	}
	return fmt.Errorf("billing event publish failed after %d attempts: %w", p.retries, lastErr)
}
