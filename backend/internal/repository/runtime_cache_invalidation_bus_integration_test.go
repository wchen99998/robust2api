//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRuntimeCacheInvalidationBus_PublishSubscribe(t *testing.T) {
	rdbSubscriber := testRedis(t)
	rdbPublisher := testRedis(t)

	subscriber := NewRuntimeCacheInvalidationBus(rdbSubscriber)
	publisher := NewRuntimeCacheInvalidationBus(rdbPublisher)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tests := []struct {
		name      string
		subscribe func(context.Context, func()) error
		publish   func(context.Context) error
	}{
		{
			name:      "settings",
			subscribe: subscriber.SubscribeSettings,
			publish:   publisher.PublishSettings,
		},
		{
			name:      "channels",
			subscribe: subscriber.SubscribeChannels,
			publish:   publisher.PublishChannels,
		},
		{
			name:      "accounts",
			subscribe: subscriber.SubscribeAccounts,
			publish:   publisher.PublishAccounts,
		},
		{
			name:      "pricing",
			subscribe: subscriber.SubscribePricing,
			publish:   publisher.PublishPricing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delivered := make(chan struct{}, 1)
			require.NoError(t, tt.subscribe(ctx, func() {
				select {
				case delivered <- struct{}{}:
				default:
				}
			}))

			require.NoError(t, tt.publish(context.Background()))
			require.Eventually(t, func() bool {
				select {
				case <-delivered:
					return true
				default:
					return false
				}
			}, 5*time.Second, 50*time.Millisecond)
		})
	}
}
