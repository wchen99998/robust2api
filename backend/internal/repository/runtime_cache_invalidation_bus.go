package repository

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/wchen99998/robust2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	settingsInvalidationChannel = "robust2api:invalidate:settings"
	channelsInvalidationChannel = "robust2api:invalidate:channels"
	accountsInvalidationChannel = "robust2api:invalidate:accounts"
	pricingInvalidationChannel  = "robust2api:invalidate:pricing"
	runtimeCacheRetryDelay      = 5 * time.Second
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

	if ctx == nil {
		ctx = context.Background()
	}

	firstErrCh := make(chan error, 1)
	firstReadyCh := make(chan struct{})

	go func() {
		readySent := false
		errSent := false

		for {
			pubsub := b.rdb.Subscribe(ctx, channel)
			if _, err := pubsub.Receive(ctx); err != nil {
				_ = pubsub.Close()
				if !readySent && !errSent {
					wrapped := fmt.Errorf("subscribe to %s: %w", channel, err)
					firstErrCh <- wrapped
					errSent = true
				} else {
					log.Printf("warning: runtime cache invalidation subscribe failed for %s: %v", channel, err)
				}
				if !waitForRuntimeCacheRetry(ctx) {
					return
				}
				continue
			}

			if !readySent {
				close(firstReadyCh)
				readySent = true
			}

			disconnected := false
			ch := pubsub.Channel()
			for !disconnected {
				select {
				case <-ctx.Done():
					disconnected = true
				case msg, ok := <-ch:
					if !ok {
						log.Printf("warning: runtime cache invalidation pubsub closed for %s; retrying", channel)
						disconnected = true
						continue
					}
					if msg != nil {
						handler()
					}
				}
			}

			if err := pubsub.Close(); err != nil {
				log.Printf("warning: failed to close runtime cache invalidation pubsub for %s: %v", channel, err)
			}

			if ctx.Err() != nil {
				return
			}
			if !waitForRuntimeCacheRetry(ctx) {
				return
			}
		}
	}()

	select {
	case <-firstReadyCh:
		return nil
	case err := <-firstErrCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func waitForRuntimeCacheRetry(ctx context.Context) bool {
	timer := time.NewTimer(runtimeCacheRetryDelay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
