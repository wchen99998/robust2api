//go:build wireinject
// +build wireinject

package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/wchen99998/robust2api/ent"
	"github.com/wchen99998/robust2api/internal/maintenance"
	platformconfig "github.com/wchen99998/robust2api/internal/platform/config"
	platformdatabase "github.com/wchen99998/robust2api/internal/platform/database"
	platformhealth "github.com/wchen99998/robust2api/internal/platform/health"
	platformotel "github.com/wchen99998/robust2api/internal/platform/otel"
	platformredis "github.com/wchen99998/robust2api/internal/platform/redis"
	"github.com/wchen99998/robust2api/internal/repository"
	"github.com/wchen99998/robust2api/internal/service"

	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
)

type Application struct {
	Health  *platformhealth.Checker
	Cleanup func()
}

func initialize() (*Application, error) {
	wire.Build(
		platformconfig.WorkerProviderSet,
		platformotel.ProviderSet,
		platformdatabase.ProviderSet,
		platformredis.ProviderSet,
		repository.AdapterProviderSet,
		maintenance.ProviderSet,
		platformhealth.ProviderSet,
		providePrivacyClientFactory,
		provideCleanup,
		wire.Struct(new(Application), "Health", "Cleanup"),
	)
	return nil, nil
}

func providePrivacyClientFactory() service.PrivacyClientFactory {
	return repository.CreatePrivacyReqClient
}

func provideCleanup(
	entClient *ent.Client,
	rdb *redis.Client,
	otelProvider *platformotel.Provider,
	metricsServer *platformotel.MetricsServer,
	_ *service.ConcurrencyService,
	schedulerSnapshot *service.SchedulerSnapshotService,
	tokenRefresh *service.TokenRefreshService,
	accountExpiry *service.AccountExpiryService,
	subscriptionExpiry *service.SubscriptionExpiryService,
	usageCleanup *service.UsageCleanupService,
	idempotencyCleanup *service.IdempotencyCleanupService,
	pricing *service.PricingService,
	scheduledTestRunner *service.ScheduledTestRunnerService,
	userMsgQueue *service.UserMessageQueueService,
	oauth *service.OAuthService,
	openaiOAuth *service.OpenAIOAuthService,
	geminiOAuth *service.GeminiOAuthService,
	antigravityOAuth *service.AntigravityOAuthService,
) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		type cleanupStep struct {
			name string
			fn   func() error
		}

		parallelSteps := []cleanupStep{
			{"SchedulerSnapshotService", func() error {
				if schedulerSnapshot != nil {
					schedulerSnapshot.Stop()
				}
				return nil
			}},
			{"TokenRefreshService", func() error {
				tokenRefresh.Stop()
				return nil
			}},
			{"AccountExpiryService", func() error {
				accountExpiry.Stop()
				return nil
			}},
			{"SubscriptionExpiryService", func() error {
				subscriptionExpiry.Stop()
				return nil
			}},
			{"UsageCleanupService", func() error {
				if usageCleanup != nil {
					usageCleanup.Stop()
				}
				return nil
			}},
			{"IdempotencyCleanupService", func() error {
				if idempotencyCleanup != nil {
					idempotencyCleanup.Stop()
				}
				return nil
			}},
			{"PricingService", func() error {
				pricing.Stop()
				return nil
			}},
			{"ScheduledTestRunnerService", func() error {
				if scheduledTestRunner != nil {
					scheduledTestRunner.Stop()
				}
				return nil
			}},
			{"UserMessageQueueService", func() error {
				if userMsgQueue != nil {
					userMsgQueue.Stop()
				}
				return nil
			}},
			{"OAuthService", func() error {
				oauth.Stop()
				return nil
			}},
			{"OpenAIOAuthService", func() error {
				openaiOAuth.Stop()
				return nil
			}},
			{"GeminiOAuthService", func() error {
				geminiOAuth.Stop()
				return nil
			}},
			{"AntigravityOAuthService", func() error {
				antigravityOAuth.Stop()
				return nil
			}},
		}

		infraSteps := []cleanupStep{
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
