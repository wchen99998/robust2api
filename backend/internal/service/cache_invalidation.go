package service

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	cacheInvalidationPublishTimeout = 5 * time.Second
	cacheInvalidationDebounce       = time.Second
)

// RuntimeCacheInvalidationBus fans out cache invalidation notices across runtime roles.
type RuntimeCacheInvalidationBus interface {
	PublishSettings(ctx context.Context) error
	PublishChannels(ctx context.Context) error
	PublishAccounts(ctx context.Context) error
	PublishPricing(ctx context.Context) error

	SubscribeSettings(ctx context.Context, handler func()) error
	SubscribeChannels(ctx context.Context, handler func()) error
	SubscribeAccounts(ctx context.Context, handler func()) error
	SubscribePricing(ctx context.Context, handler func()) error
}

// GatewayCacheInvalidationSubscribers marks gateway runtime cache subscribers as started.
type GatewayCacheInvalidationSubscribers struct{}

// ControlCacheInvalidationSubscribers marks control-plane runtime cache subscribers as started.
type ControlCacheInvalidationSubscribers struct{}

func publishInvalidation(scope string, publish func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), cacheInvalidationPublishTimeout)
	defer cancel()

	if err := publish(ctx); err != nil {
		slog.Warn("runtime cache invalidation publish failed", "scope", scope, "error", err)
	}
}

func newDebouncedInvalidationHandler(delay time.Duration, handler func()) func() {
	if handler == nil {
		return func() {}
	}

	if delay <= 0 {
		var mu sync.Mutex
		return func() {
			mu.Lock()
			defer mu.Unlock()
			handler()
		}
	}

	type debouncer struct {
		delay   time.Duration
		handler func()

		mu      sync.Mutex
		timer   *time.Timer
		pending bool
		running bool
	}

	d := &debouncer{
		delay:   delay,
		handler: handler,
		timer:   time.NewTimer(delay),
	}
	if !d.timer.Stop() {
		select {
		case <-d.timer.C:
		default:
		}
	}

	go func() {
		for range d.timer.C {
			d.mu.Lock()
			if !d.pending || d.running {
				d.mu.Unlock()
				continue
			}
			d.pending = false
			d.running = true
			d.mu.Unlock()

			d.handler()

			d.mu.Lock()
			d.running = false
			if d.pending {
				if !d.timer.Stop() {
					select {
					case <-d.timer.C:
					default:
					}
				}
				d.timer.Reset(d.delay)
			}
			d.mu.Unlock()
		}
	}()

	return func() {
		d.mu.Lock()
		defer d.mu.Unlock()

		d.pending = true
		if d.running {
			return
		}
		if !d.timer.Stop() {
			select {
			case <-d.timer.C:
			default:
			}
		}
		d.timer.Reset(d.delay)
	}
}

func subscribeInvalidation(scope string, subscribe func(context.Context, func()) error, handler func()) {
	if subscribe == nil || handler == nil {
		return
	}
	if err := subscribe(context.Background(), newDebouncedInvalidationHandler(cacheInvalidationDebounce, handler)); err != nil {
		slog.Warn("runtime cache invalidation subscribe failed", "scope", scope, "error", err)
	}
}

// ProvideGatewayCacheInvalidationSubscribers warms gateway-side caches and starts pub/sub listeners.
func ProvideGatewayCacheInvalidationSubscribers(
	bus RuntimeCacheInvalidationBus,
	settingService *SettingService,
	channelService *ChannelService,
	schedulerSnapshot *SchedulerSnapshotService,
	pricing *PricingService,
) *GatewayCacheInvalidationSubscribers {
	if settingService != nil {
		if err := settingService.RefreshGatewayHotPathCache(context.Background()); err != nil {
			slog.Warn("gateway hot settings warmup failed", "error", err)
		}
	}
	if channelService != nil {
		if err := channelService.RefreshCache(context.Background()); err != nil {
			slog.Warn("gateway channel cache warmup failed", "error", err)
		}
	}
	if schedulerSnapshot != nil {
		if err := schedulerSnapshot.TriggerFullRebuildNow("gateway_startup"); err != nil {
			slog.Warn("gateway scheduler warmup failed", "error", err)
		}
	}
	if bus == nil {
		return &GatewayCacheInvalidationSubscribers{}
	}

	subscribeInvalidation("settings", bus.SubscribeSettings, func() {
		if settingService == nil {
			return
		}
		if err := settingService.RefreshGatewayHotPathCache(context.Background()); err != nil {
			slog.Warn("gateway hot settings refresh failed", "error", err)
		}
	})
	subscribeInvalidation("channels", bus.SubscribeChannels, func() {
		if channelService == nil {
			return
		}
		if err := channelService.RefreshCache(context.Background()); err != nil {
			slog.Warn("gateway channel cache refresh failed", "error", err)
		}
	})
	subscribeInvalidation("accounts", bus.SubscribeAccounts, func() {
		if schedulerSnapshot == nil {
			return
		}
		if err := schedulerSnapshot.TriggerFullRebuildNow("pubsub_accounts"); err != nil {
			slog.Warn("gateway scheduler refresh failed", "error", err)
		}
	})
	subscribeInvalidation("pricing", bus.SubscribePricing, func() {
		if pricing == nil {
			return
		}
		if err := pricing.ForceUpdate(); err != nil {
			slog.Warn("gateway pricing refresh failed", "error", err)
		}
	})

	return &GatewayCacheInvalidationSubscribers{}
}

// ProvideControlCacheInvalidationSubscribers warms control-plane caches and starts pub/sub listeners.
func ProvideControlCacheInvalidationSubscribers(
	bus RuntimeCacheInvalidationBus,
	settingService *SettingService,
	channelService *ChannelService,
	pricing *PricingService,
) *ControlCacheInvalidationSubscribers {
	if settingService != nil {
		if err := settingService.RefreshGatewayHotPathCache(context.Background()); err != nil {
			slog.Warn("control hot settings warmup failed", "error", err)
		}
	}
	if channelService != nil {
		if err := channelService.RefreshCache(context.Background()); err != nil {
			slog.Warn("control channel cache warmup failed", "error", err)
		}
	}
	if bus == nil {
		return &ControlCacheInvalidationSubscribers{}
	}

	subscribeInvalidation("settings", bus.SubscribeSettings, func() {
		if settingService == nil {
			return
		}
		if err := settingService.RefreshGatewayHotPathCache(context.Background()); err != nil {
			slog.Warn("control hot settings refresh failed", "error", err)
		}
	})
	subscribeInvalidation("channels", bus.SubscribeChannels, func() {
		if channelService == nil {
			return
		}
		if err := channelService.RefreshCache(context.Background()); err != nil {
			slog.Warn("control channel cache refresh failed", "error", err)
		}
	})
	subscribeInvalidation("pricing", bus.SubscribePricing, func() {
		if pricing == nil {
			return
		}
		if err := pricing.ForceUpdate(); err != nil {
			slog.Warn("control pricing refresh failed", "error", err)
		}
	})

	return &ControlCacheInvalidationSubscribers{}
}
