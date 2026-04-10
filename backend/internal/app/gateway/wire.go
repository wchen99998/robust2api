//go:build wireinject
// +build wireinject

package gateway

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/gatewayruntime"
	platformconfig "github.com/Wei-Shaw/sub2api/internal/platform/config"
	platformdatabase "github.com/Wei-Shaw/sub2api/internal/platform/database"
	platformhealth "github.com/Wei-Shaw/sub2api/internal/platform/health"
	platformhttp "github.com/Wei-Shaw/sub2api/internal/platform/httpserver"
	platformotel "github.com/Wei-Shaw/sub2api/internal/platform/otel"
	platformredis "github.com/Wei-Shaw/sub2api/internal/platform/redis"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
)

type Application struct {
	Server        *http.Server
	MetricsServer *platformotel.MetricsServer
	Health        *platformhealth.Checker
	Cleanup       func()
}

func initialize() (*Application, error) {
	wire.Build(
		platformconfig.GatewayProviderSet,
		platformotel.ProviderSet,
		platformdatabase.ProviderSet,
		platformredis.ProviderSet,
		repository.AdapterProviderSet,
		gatewayruntime.ProviderSet,
		platformhealth.ProviderSet,
		platformhttp.ProviderSet,
		provideCleanup,
		wire.Struct(new(Application), "Server", "MetricsServer", "Health", "Cleanup"),
	)
	return nil, nil
}

func provideCleanup(
	entClient *ent.Client,
	rdb *redis.Client,
	otelProvider *platformotel.Provider,
	metricsServer *platformotel.MetricsServer,
	billingCache *service.BillingCacheService,
	usageRecordWorkerPool *service.UsageRecordWorkerPool,
	subscriptionService *service.SubscriptionService,
	pricing *service.PricingService,
	deferred *service.DeferredService,
	oauth *service.OAuthService,
	openaiOAuth *service.OpenAIOAuthService,
	geminiOAuth *service.GeminiOAuthService,
	antigravityOAuth *service.AntigravityOAuthService,
	openAIGateway *service.OpenAIGatewayService,
) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		type cleanupStep struct {
			name string
			fn   func() error
		}

		parallelSteps := []cleanupStep{
			{"BillingCacheService", func() error {
				if billingCache != nil {
					billingCache.Stop()
				}
				return nil
			}},
			{"UsageRecordWorkerPool", func() error {
				if usageRecordWorkerPool != nil {
					usageRecordWorkerPool.Stop()
				}
				return nil
			}},
			{"SubscriptionService", func() error {
				if subscriptionService != nil {
					subscriptionService.Stop()
				}
				return nil
			}},
			{"DeferredService", func() error {
				if deferred != nil {
					deferred.Stop()
				}
				return nil
			}},
			{"PricingService", func() error {
				if pricing != nil {
					pricing.Stop()
				}
				return nil
			}},
			{"OAuthService", func() error {
				if oauth != nil {
					oauth.Stop()
				}
				return nil
			}},
			{"OpenAIOAuthService", func() error {
				if openaiOAuth != nil {
					openaiOAuth.Stop()
				}
				return nil
			}},
			{"GeminiOAuthService", func() error {
				if geminiOAuth != nil {
					geminiOAuth.Stop()
				}
				return nil
			}},
			{"AntigravityOAuthService", func() error {
				if antigravityOAuth != nil {
					antigravityOAuth.Stop()
				}
				return nil
			}},
			{"OpenAIWSPool", func() error {
				if openAIGateway != nil {
					openAIGateway.CloseOpenAIWSPool()
				}
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
	}
}
