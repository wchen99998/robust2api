package repository

import (
	"context"
	"fmt"
	"log"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	settingsInvalidationChannel = "sub2api:invalidate:settings"
	channelsInvalidationChannel = "sub2api:invalidate:channels"
	accountsInvalidationChannel = "sub2api:invalidate:accounts"
	pricingInvalidationChannel  = "sub2api:invalidate:pricing"
)

type runtimeCacheInvalidationBus struct {
	rdb *redis.Client
}

func NewRuntimeCacheInvalidationBus(rdb *redis.Client) service.RuntimeCacheInvalidationBus {
	return &runtimeCacheInvalidationBus{rdb: rdb}
}

func (b *runtimeCacheInvalidationBus) PublishSettings(ctx context.Context) error {
	return b.publish(ctx, settingsInvalidationChannel)
}

func (b *runtimeCacheInvalidationBus) PublishChannels(ctx context.Context) error {
	return b.publish(ctx, channelsInvalidationChannel)
}

func (b *runtimeCacheInvalidationBus) PublishAccounts(ctx context.Context) error {
	return b.publish(ctx, accountsInvalidationChannel)
}

func (b *runtimeCacheInvalidationBus) PublishPricing(ctx context.Context) error {
	return b.publish(ctx, pricingInvalidationChannel)
}

func (b *runtimeCacheInvalidationBus) SubscribeSettings(ctx context.Context, handler func()) error {
	return b.subscribe(ctx, settingsInvalidationChannel, handler)
}

func (b *runtimeCacheInvalidationBus) SubscribeChannels(ctx context.Context, handler func()) error {
	return b.subscribe(ctx, channelsInvalidationChannel, handler)
}

func (b *runtimeCacheInvalidationBus) SubscribeAccounts(ctx context.Context, handler func()) error {
	return b.subscribe(ctx, accountsInvalidationChannel, handler)
}

func (b *runtimeCacheInvalidationBus) SubscribePricing(ctx context.Context, handler func()) error {
	return b.subscribe(ctx, pricingInvalidationChannel, handler)
}

func (b *runtimeCacheInvalidationBus) publish(ctx context.Context, channel string) error {
	if b == nil || b.rdb == nil {
		return nil
	}
	return b.rdb.Publish(ctx, channel, "refresh").Err()
}

func (b *runtimeCacheInvalidationBus) subscribe(ctx context.Context, channel string, handler func()) error {
	if b == nil || b.rdb == nil || handler == nil {
		return nil
	}

	pubsub := b.rdb.Subscribe(ctx, channel)
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return fmt.Errorf("subscribe to %s: %w", channel, err)
	}

	go func() {
		defer func() {
			if err := pubsub.Close(); err != nil {
				log.Printf("warning: failed to close runtime cache invalidation pubsub for %s: %v", channel, err)
			}
		}()

		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				if msg != nil {
					handler()
				}
			}
		}
	}()

	return nil
}
