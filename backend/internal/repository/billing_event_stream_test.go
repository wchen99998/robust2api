package repository

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestNewRedisBillingEventPublisherUsesPublishTimeoutConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Billing.Stream.PublishRetries = 5
	cfg.Billing.Stream.PublishTimeoutSeconds = 7

	publisher := NewRedisBillingEventPublisher(nil, cfg)
	impl, ok := publisher.(*redisBillingEventPublisher)
	require.True(t, ok)
	require.Equal(t, 5, impl.retries)
	require.Equal(t, 7*time.Second, impl.publishTimeout)
}

func TestRedisBillingEventPublisherPublishHonorsAttemptTimeout(t *testing.T) {
	t.Parallel()

	rdb := redis.NewClient(&redis.Options{
		Addr: "billing.test:6379",
		Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	publisher := &redisBillingEventPublisher{
		rdb:            rdb,
		key:            "billing:events",
		retries:        1,
		publishTimeout: 25 * time.Millisecond,
	}

	startedAt := time.Now()
	err := publisher.Publish(context.Background(), &service.UsageChargeEvent{
		Version:   2,
		EventID:   "evt-timeout",
		RequestID: "req-timeout",
	})
	elapsed := time.Since(startedAt)

	require.Error(t, err)
	require.ErrorContains(t, err, "context deadline exceeded")
	require.Less(t, elapsed, 250*time.Millisecond)
}
