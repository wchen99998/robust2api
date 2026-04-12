//go:build wireinject
// +build wireinject

package billing

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	platformconfig "github.com/Wei-Shaw/sub2api/internal/platform/config"
	platformdatabase "github.com/Wei-Shaw/sub2api/internal/platform/database"
	platformhealth "github.com/Wei-Shaw/sub2api/internal/platform/health"
	platformotel "github.com/Wei-Shaw/sub2api/internal/platform/otel"
	platformredis "github.com/Wei-Shaw/sub2api/internal/platform/redis"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
)

type Application struct {
	Health          *platformhealth.Checker
	BillingConsumer *service.BillingConsumerService
	Cleanup         func()
}

func initialize() (*Application, error) {
	wire.Build(
		platformconfig.BillingProviderSet,
		platformotel.ProviderSet,
		platformdatabase.ProviderSet,
		platformredis.ProviderSet,
		repository.AdapterProviderSet,
		service.BillingConsumerProviderSet,
		platformhealth.ProviderSet,
		provideCleanup,
		wire.Struct(new(Application), "Health", "BillingConsumer", "Cleanup"),
	)
	return nil, nil
}

func provideCleanup(
	entClient *ent.Client,
	rdb *redis.Client,
	otelProvider *platformotel.Provider,
	metricsServer *platformotel.MetricsServer,
	billingDB *repository.BillingDB,
	billingConsumer *service.BillingConsumerService,
	billingCache *service.BillingCacheService,
	deferredService *service.DeferredService,
) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		type cleanupStep struct {
			name string
			fn   func() error
		}

		parallelSteps := []cleanupStep{
			{"BillingConsumerService", func() error {
				if billingConsumer != nil {
					billingConsumer.Stop()
				}
				return nil
			}},
			{"BillingCacheService", func() error {
				if billingCache != nil {
					billingCache.Stop()
				}
				return nil
			}},
			{"DeferredService", func() error {
				if deferredService != nil {
					deferredService.Stop()
				}
				return nil
			}},
		}

		infraSteps := []cleanupStep{
			{"BillingDB", func() error {
				if billingDB != nil {
					return billingDB.Close()
				}
				return nil
			}},
			{"Redis", func() error {
				if rdb == nil {
					return nil
				}
				return rdb.Close()
			}},
			{"Ent", func() error {
				if entClient == nil {
					return nil
				}
				return entClient.Close()
			}},
		}

		runParallel := func(steps []cleanupStep) {
			var wg sync.WaitGroup
			for i := range steps {
				step := steps[i]
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := step.fn(); err != nil {
						log.Printf("[Cleanup] %s failed: %v", step.name, err)
						return
					}
					log.Printf("[Cleanup] %s succeeded", step.name)
				}()
			}
			wg.Wait()
		}

		runSequential := func(steps []cleanupStep) {
			for i := range steps {
				step := steps[i]
				if err := step.fn(); err != nil {
					log.Printf("[Cleanup] %s failed: %v", step.name, err)
					continue
				}
				log.Printf("[Cleanup] %s succeeded", step.name)
			}
		}

		runParallel(parallelSteps)

		if otelProvider != nil {
			if err := otelProvider.Shutdown(ctx); err != nil {
				log.Printf("OTel provider shutdown error: %v", err)
			}
		}
		if metricsServer != nil {
			if err := metricsServer.Shutdown(ctx); err != nil {
				log.Printf("Metrics server shutdown error: %v", err)
			}
		}

		runSequential(infraSteps)

		select {
		case <-ctx.Done():
			log.Printf("[Cleanup] Warning: cleanup timed out after 30 seconds")
		default:
			log.Printf("[Cleanup] All cleanup steps completed")
		}
	}
}
