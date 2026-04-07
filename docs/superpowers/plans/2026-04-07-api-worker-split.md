# API/Worker Split Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the monolithic `cmd/server/` binary into separate `cmd/api/` and `cmd/worker/` binaries with distinct Wire injectors, Docker images, and Helm deployments.

**Architecture:** Restructure `service.ProviderSet` into `SharedProviderSet` (pure constructors), `APIProviderSet` (shared + API-specific `.Start()` calls), and `WorkerProviderSet` (shared + worker-specific `.Start()` calls). Each binary gets its own Wire injector, `main.go`, cleanup function, and health probes. Helm chart splits the single Deployment into `api-deployment` + `worker-deployment`.

**Tech Stack:** Go (Wire DI, Gin), Helm 3, GoReleaser, Docker multi-stage builds

---

### Task 1: Split `service.ProviderSet` into Shared/API/Worker sets

**Files:**
- Modify: `backend/internal/service/wire.go`

This is the core change. The current `ProviderSet` (line 307–382) mixes pure constructors with providers that call `.Start()`. We split it into three sets.

- [ ] **Step 1: Define `SharedProviderSet` with pure constructors only**

In `backend/internal/service/wire.go`, replace the single `ProviderSet` variable (lines 307–382) with three provider sets. `SharedProviderSet` contains all entries from the old `ProviderSet` EXCEPT the ones that call `.Start()` or start goroutines. Remove these from `SharedProviderSet`:

- `ProvideAPIKeyAuthCacheInvalidator` (starts Pub/Sub subscriber)
- `ProvideTokenRefreshService` (calls `.Start()`)
- `ProvideDashboardAggregationService` (calls `.Start()`)
- `ProvideUsageCleanupService` (calls `.Start()`)
- `ProvideAccountExpiryService` (calls `.Start()`)
- `ProvideSubscriptionExpiryService` (calls `.Start()`)
- `ProvideTimingWheelService` (calls `.Start()`)
- `ProvideDeferredService` (calls `.Start()`)
- `ProvideConcurrencyService` (calls `.StartSlotCleanupWorker()`)
- `ProvideUserMessageQueueService` (calls `.StartCleanupWorker()`)
- `ProvideSchedulerSnapshotService` (calls `.Start()`)
- `ProvideIdempotencyCleanupService` (calls `.Start()`)
- `ProvideScheduledTestRunnerService` (calls `.Start()`)
- `ProvideBackupService` (calls `.Start()`)
- `ProvidePricingService` (calls `.Initialize()` — needs role-specific variants)
- `ProvideEmailQueueService` (starts workers in constructor)
- `NewBillingCacheService` (starts workers in constructor)
- `NewUsageRecordWorkerPool` (needs explicit Start for API)

Replace with new pure-constructor variants where needed:

```go
// SharedProviderSet contains pure constructors with no side effects.
// No goroutines are started, no .Start() is called.
var SharedProviderSet = wire.NewSet(
	// Core services (pure constructors)
	NewAuthService,
	NewUserService,
	NewAPIKeyService,
	NewGroupService,
	NewAccountService,
	NewProxyService,
	NewRedeemService,
	NewPromoService,
	NewUsageService,
	NewDashboardService,
	NewBillingService,
	NewAnnouncementService,
	NewAdminService,
	NewGatewayService,
	NewOpenAIGatewayService,
	NewOAuthService,
	NewOpenAIOAuthService,
	NewGeminiOAuthService,
	NewGeminiQuotaService,
	NewCompositeTokenCacheInvalidator,
	wire.Bind(new(TokenCacheInvalidator), new(*CompositeTokenCacheInvalidator)),
	NewAntigravityOAuthService,
	NewOAuthRefreshAPI,
	ProvideGeminiTokenProvider,
	NewGeminiMessagesCompatService,
	ProvideAntigravityTokenProvider,
	ProvideOpenAITokenProvider,
	ProvideClaudeTokenProvider,
	NewAntigravityGatewayService,
	ProvideRateLimitService,
	NewAccountUsageService,
	NewAccountTestService,
	ProvideSettingService,
	NewDataManagementService,
	NewOpsService,
	NewEmailService,
	NewTurnstileService,
	NewSubscriptionService,
	wire.Bind(new(DefaultSubscriptionAssigner), new(*SubscriptionService)),
	NewIdentityService,
	NewCRSSyncService,
	ProvideUpdateService,
	NewAntigravityQuotaFetcher,
	NewUserAttributeService,
	NewUsageCache,
	NewTotpService,
	NewErrorPassthroughService,
	NewTLSFingerprintProfileService,
	NewDigestSessionStore,
	ProvideIdempotencyCoordinator,
	ProvideSystemOperationLockService,
	ProvideScheduledTestService,
	NewGroupCapacityService,
	NewChannelService,
	NewModelPricingResolver,
)
```

- [ ] **Step 2: Create role-specific provider functions**

Add these new provider functions to `backend/internal/service/wire.go`:

```go
// --- API-specific providers ---

// ProvideAPITimingWheelService creates TimingWheelService without starting it.
// API role does not need the timing wheel's background tick.
func ProvideAPITimingWheelService() (*TimingWheelService, error) {
	return NewTimingWheelService()
}

// ProvideAPIDeferredService creates DeferredService without starting it.
func ProvideAPIDeferredService(accountRepo AccountRepository, timingWheel *TimingWheelService) *DeferredService {
	return NewDeferredService(accountRepo, timingWheel, 10*time.Second)
}

// ProvideAPIPricingService creates PricingService with Initialize() but no update scheduler.
func ProvideAPIPricingService(cfg *config.Config, remoteClient PricingRemoteClient) (*PricingService, error) {
	svc := NewPricingService(cfg, remoteClient)
	if err := svc.Initialize(); err != nil {
		println("[Service] Warning: Pricing service initialization failed:", err.Error())
	}
	return svc, nil
}

// ProvideAPISchedulerSnapshotService creates SchedulerSnapshotService without starting background workers.
// API reads snapshots from Redis; worker keeps them fresh.
func ProvideAPISchedulerSnapshotService(
	cache SchedulerCache,
	outboxRepo SchedulerOutboxRepository,
	accountRepo AccountRepository,
	groupRepo GroupRepository,
	cfg *config.Config,
) *SchedulerSnapshotService {
	return NewSchedulerSnapshotService(cache, outboxRepo, accountRepo, groupRepo, cfg)
}

// ProvideAPIConcurrencyService creates ConcurrencyService without the cleanup worker.
// API only needs slot acquire/release; worker runs cleanup.
func ProvideAPIConcurrencyService(cache ConcurrencyCache, accountRepo AccountRepository, cfg *config.Config) *ConcurrencyService {
	svc := NewConcurrencyService(cache)
	if err := svc.CleanupStaleProcessSlots(context.Background()); err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: startup cleanup stale process slots failed: %v", err)
	}
	return svc
}

// ProvideAPIUserMessageQueueService creates UserMessageQueueService without cleanup worker.
func ProvideAPIUserMessageQueueService(cache UserMsgQueueCache, rpmCache RPMCache, cfg *config.Config) *UserMessageQueueService {
	return NewUserMessageQueueService(cache, rpmCache, &cfg.Gateway.UserMessageQueue)
}

// ProvideAPIBillingCacheService creates and starts BillingCacheService (request-path async workers).
func ProvideAPIBillingCacheService(cache BillingCache, userRepo UserRepository, subRepo UserSubscriptionRepository, apiKeyRepo APIKeyRepository, cfg *config.Config) *BillingCacheService {
	return NewBillingCacheService(cache, userRepo, subRepo, apiKeyRepo, cfg)
}

// ProvideAPIUsageRecordWorkerPool creates UsageRecordWorkerPool (request-path async workers).
func ProvideAPIUsageRecordWorkerPool(cfg *config.Config) *UsageRecordWorkerPool {
	return NewUsageRecordWorkerPool(cfg)
}

// ProvideAPIEmailQueueService creates EmailQueueService with workers (request-path async).
func ProvideAPIEmailQueueService(emailService *EmailService) *EmailQueueService {
	return NewEmailQueueService(emailService, 3)
}

// ProvideAPIIdempotencyCleanupService creates IdempotencyCleanupService without starting its loop.
// Worker will own the cleanup loop.
func ProvideAPIIdempotencyCleanupService(repo IdempotencyRepository, cfg *config.Config) *IdempotencyCleanupService {
	return NewIdempotencyCleanupService(repo, cfg)
}

// --- Worker-specific providers ---

// ProvideWorkerConcurrencyService creates ConcurrencyService with the cleanup worker.
func ProvideWorkerConcurrencyService(cache ConcurrencyCache, accountRepo AccountRepository, cfg *config.Config) *ConcurrencyService {
	svc := NewConcurrencyService(cache)
	if err := svc.CleanupStaleProcessSlots(context.Background()); err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: startup cleanup stale process slots failed: %v", err)
	}
	if cfg != nil {
		svc.StartSlotCleanupWorker(accountRepo, cfg.Gateway.Scheduling.SlotCleanupInterval)
	}
	return svc
}

// ProvideWorkerPricingService creates PricingService with Initialize() + update scheduler.
func ProvideWorkerPricingService(cfg *config.Config, remoteClient PricingRemoteClient) (*PricingService, error) {
	svc := NewPricingService(cfg, remoteClient)
	if err := svc.Initialize(); err != nil {
		println("[Service] Warning: Pricing service initialization failed:", err.Error())
	}
	return svc, nil
}
```

Note: `ProvideWorkerPricingService` is identical to the current `ProvidePricingService` for now. The `PricingService.Initialize()` method already starts the update scheduler internally. If it doesn't, we'll call it explicitly after reading the actual `Initialize()` implementation during task execution.

- [ ] **Step 3: Define `APIProviderSet`**

```go
// APIProviderSet is SharedProviderSet + API-specific providers.
// Includes request-path async workers and cache invalidation.
var APIProviderSet = wire.NewSet(
	SharedProviderSet,

	// API starts these (request-path async continuations)
	ProvideAPIBillingCacheService,
	ProvideAPIUsageRecordWorkerPool,
	ProvideAPIEmailQueueService,
	ProvideAPIKeyAuthCacheInvalidator,

	// API uses read-only versions (no background loops)
	ProvideAPIPricingService,
	ProvideAPISchedulerSnapshotService,
	ProvideAPIConcurrencyService,
	ProvideAPIUserMessageQueueService,
	ProvideAPITimingWheelService,
	ProvideAPIDeferredService,
	ProvideAPIIdempotencyCleanupService,

	// Worker-only services not needed by API — but Wire requires them
	// if any shared service depends on them transitively.
	// We provide no-op / non-started versions where needed.
)
```

- [ ] **Step 4: Define `WorkerProviderSet`**

```go
// WorkerProviderSet is SharedProviderSet + worker-specific providers.
// Includes all background maintenance loops.
var WorkerProviderSet = wire.NewSet(
	SharedProviderSet,

	// Worker starts all maintenance loops
	ProvideTokenRefreshService,
	ProvideDashboardAggregationService,
	ProvideUsageCleanupService,
	ProvideAccountExpiryService,
	ProvideSubscriptionExpiryService,
	ProvideTimingWheelService,
	ProvideDeferredService,
	ProvideSchedulerSnapshotService,
	ProvideIdempotencyCleanupService,
	ProvideScheduledTestRunnerService,
	ProvideBackupService,
	ProvideWorkerPricingService,
	ProvideWorkerConcurrencyService,
	ProvideUserMessageQueueService,

	// Worker also needs these for transitive deps (but doesn't serve HTTP)
	ProvideEmailQueueService,
	NewBillingCacheService,
	NewUsageRecordWorkerPool,

	// Worker doesn't use API key auth cache invalidation
	// but some shared services may depend on APIKeyAuthCacheInvalidator interface.
	// Provide a no-op or the real one depending on dependency analysis.
	ProvideAPIKeyAuthCacheInvalidator,
)
```

- [ ] **Step 5: Keep the old `ProviderSet` as a deprecated alias (temporary)**

```go
// ProviderSet is DEPRECATED — use APIProviderSet or WorkerProviderSet.
// Kept temporarily for compilation during migration.
var ProviderSet = APIProviderSet
```

- [ ] **Step 6: Verify compilation**

Run:
```bash
cd backend && go build ./internal/service/...
```
Expected: compiles without errors. Wire generation will happen in later tasks.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/service/wire.go
git commit -m "refactor(service): split ProviderSet into Shared/API/Worker sets

Separate pure constructors (SharedProviderSet) from providers that
start background goroutines. APIProviderSet adds request-path async
workers and cache invalidation. WorkerProviderSet adds all maintenance
loops and data refresh workers."
```

---

### Task 2: Create health probe infrastructure

**Files:**
- Create: `backend/internal/health/health.go`

Both API and Worker need health probes. Create a shared package.

- [ ] **Step 1: Create the health probe package**

Create `backend/internal/health/health.go`:

```go
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/redis/go-redis/v9"
)

// Checker provides health probe endpoints.
type Checker struct {
	entClient *ent.Client
	rdb       *redis.Client
	ready     atomic.Bool
}

// NewChecker creates a Checker with DB and Redis clients.
func NewChecker(entClient *ent.Client, rdb *redis.Client) *Checker {
	return &Checker{
		entClient: entClient,
		rdb:       rdb,
	}
}

// SetReady marks the service as ready to receive traffic.
func (c *Checker) SetReady() {
	c.ready.Store(true)
}

// Livez always returns 200 — process is alive.
func (c *Checker) Livez(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// Readyz returns 200 if DB and Redis are reachable, 503 otherwise.
func (c *Checker) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	type checkResult struct {
		name string
		ok   bool
	}

	results := make([]checkResult, 0, 2)

	// Check PostgreSQL
	if c.entClient != nil {
		db := c.entClient.DB()
		if db != nil {
			err := db.PingContext(ctx)
			results = append(results, checkResult{"postgresql", err == nil})
		} else {
			results = append(results, checkResult{"postgresql", false})
		}
	}

	// Check Redis
	if c.rdb != nil {
		err := c.rdb.Ping(ctx).Err()
		results = append(results, checkResult{"redis", err == nil})
	}

	allOK := true
	checks := make(map[string]string, len(results))
	for _, r := range results {
		if r.ok {
			checks[r.name] = "ok"
		} else {
			checks[r.name] = "fail"
			allOK = false
		}
	}

	resp := map[string]any{
		"status": "ok",
		"checks": checks,
	}
	if !allOK {
		resp["status"] = "fail"
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// Startupz returns 200 if SetReady() has been called, 503 otherwise.
func (c *Checker) Startupz(w http.ResponseWriter, _ *http.Request) {
	if c.ready.Load() {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"starting"}`))
	}
}

// RegisterOnMux registers all probe endpoints on a standard http.ServeMux.
// Used by the worker's internal health server.
func (c *Checker) RegisterOnMux(mux *http.ServeMux) {
	mux.HandleFunc("/livez", c.Livez)
	mux.HandleFunc("/readyz", c.Readyz)
	mux.HandleFunc("/startupz", c.Startupz)
	mux.HandleFunc("/health", c.Readyz) // transitional alias
}
```

- [ ] **Step 2: Verify compilation**

Run:
```bash
cd backend && go build ./internal/health/...
```
Expected: compiles without errors.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/health/
git commit -m "feat(health): add shared health probe package

Provides /livez, /readyz, /startupz endpoints for both API and Worker
roles. Readyz checks PostgreSQL and Redis connectivity. Startupz gates
on explicit SetReady() call after initialization completes."
```

---

### Task 3: Create `cmd/api/` entry point

**Files:**
- Create: `backend/cmd/api/main.go`
- Create: `backend/cmd/api/wire.go`
- Copy: `backend/cmd/server/VERSION` → `backend/cmd/api/VERSION`

- [ ] **Step 1: Copy VERSION file**

```bash
cp backend/cmd/server/VERSION backend/cmd/api/VERSION
```

- [ ] **Step 2: Create `cmd/api/main.go`**

Create `backend/cmd/api/main.go` (adapted from `cmd/server/main.go`):

```go
package main

//go:generate go run github.com/google/wire/cmd/wire

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

//go:embed VERSION
var embeddedVersion string

var (
	Version   = ""
	Commit    = "unknown"
	Date      = "unknown"
	BuildType = "source"
)

func init() {
	if strings.TrimSpace(Version) != "" {
		return
	}
	Version = strings.TrimSpace(embeddedVersion)
	if Version == "" {
		Version = "0.0.0-dev"
	}
}

func main() {
	logger.InitBootstrap()
	defer logger.Sync()

	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		log.Printf("Sub2API API %s (commit: %s, built: %s)\n", Version, Commit, Date)
		return
	}

	runAPIServer()
}

func runAPIServer() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if err := logger.Init(logger.OptionsFromConfig(cfg.Log)); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	if cfg.RunMode == config.RunModeSimple {
		log.Println("⚠️  WARNING: Running in SIMPLE mode - billing and quota checks are DISABLED")
	}

	buildInfo := handler.BuildInfo{
		Version:   Version,
		BuildType: BuildType,
	}

	app, err := initializeAPIApplication(buildInfo)
	if err != nil {
		log.Fatalf("Failed to initialize API application: %v", err)
	}
	defer app.Cleanup()

	// Mark ready for startup probe
	app.Health.SetReady()

	go func() {
		if err := app.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Failed to start API server: %v", err)
		}
	}()

	if app.MetricsServer != nil {
		go func() {
			if err := app.MetricsServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("Metrics server error: %v", err)
			}
		}()
	}

	log.Printf("API server started on %s", app.Server.Addr)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down API server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Server.Shutdown(ctx); err != nil {
		log.Fatalf("API server forced to shutdown: %v", err)
	}

	if app.MetricsServer != nil {
		if err := app.MetricsServer.Shutdown(ctx); err != nil {
			log.Printf("Metrics server forced to shutdown: %v", err)
		}
	}

	log.Println("API server exited")
}
```

- [ ] **Step 3: Create `cmd/api/wire.go`**

Create `backend/cmd/api/wire.go`:

```go
//go:build wireinject
// +build wireinject

package main

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/health"
	appelotel "github.com/Wei-Shaw/sub2api/internal/pkg/otel"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/server"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
)

type APIApplication struct {
	Server        *http.Server
	MetricsServer *appelotel.MetricsServer
	Health        *health.Checker
	Cleanup       func()
}

func initializeAPIApplication(buildInfo handler.BuildInfo) (*APIApplication, error) {
	wire.Build(
		config.ProviderSet,
		appelotel.ProviderSet,
		repository.ProviderSet,
		service.APIProviderSet,
		middleware.ProviderSet,
		handler.ProviderSet,
		server.ProviderSet,

		providePrivacyClientFactory,
		provideServiceBuildInfo,
		health.NewChecker,
		provideAPICleanup,

		wire.Struct(new(APIApplication), "*"),
	)
	return nil, nil
}

func providePrivacyClientFactory() service.PrivacyClientFactory {
	return repository.CreatePrivacyReqClient
}

func provideServiceBuildInfo(buildInfo handler.BuildInfo) service.BuildInfo {
	return service.BuildInfo{
		Version:   buildInfo.Version,
		BuildType: buildInfo.BuildType,
	}
}

func provideAPICleanup(
	entClient *ent.Client,
	rdb *redis.Client,
	otelProvider *appelotel.Provider,
	metricsServer *appelotel.MetricsServer,
	emailQueue *service.EmailQueueService,
	billingCache *service.BillingCacheService,
	usageRecordWorkerPool *service.UsageRecordWorkerPool,
	subscriptionService *service.SubscriptionService,
	oauth *service.OAuthService,
	openaiOAuth *service.OpenAIOAuthService,
	geminiOAuth *service.GeminiOAuthService,
	antigravityOAuth *service.AntigravityOAuthService,
	openAIGateway *service.OpenAIGatewayService,
	pricing *service.PricingService,
) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		type cleanupStep struct {
			name string
			fn   func() error
		}

		parallelSteps := []cleanupStep{
			{"EmailQueueService", func() error {
				emailQueue.Stop()
				return nil
			}},
			{"BillingCacheService", func() error {
				billingCache.Stop()
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
			{"PricingService", func() error {
				pricing.Stop()
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

		var wg sync.WaitGroup
		for i := range parallelSteps {
			step := parallelSteps[i]
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := step.fn(); err != nil {
					log.Printf("[API Cleanup] %s failed: %v", step.name, err)
					return
				}
				log.Printf("[API Cleanup] %s succeeded", step.name)
			}()
		}
		wg.Wait()

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

		for _, step := range infraSteps {
			if err := step.fn(); err != nil {
				log.Printf("[API Cleanup] %s failed: %v", step.name, err)
				continue
			}
			log.Printf("[API Cleanup] %s succeeded", step.name)
		}

		select {
		case <-ctx.Done():
			log.Printf("[API Cleanup] Warning: cleanup timed out after 10 seconds")
		default:
			log.Printf("[API Cleanup] All cleanup steps completed")
		}
	}
}
```

- [ ] **Step 4: Register health probes on the Gin router**

Modify `backend/internal/server/routes/common.go` to add the new probe endpoints. Add after the existing `/health` route:

```go
// RegisterCommonRoutes 注册通用路由（健康检查、状态等）
func RegisterCommonRoutes(r *gin.Engine) {
	// Legacy health check (transitional alias for /readyz)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Kubernetes probe endpoints
	r.GET("/livez", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Claude Code telemetry (ignored)
	r.POST("/api/event_logging/batch", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Setup status endpoint
	r.GET("/setup/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{
				"needs_setup": false,
				"step":        "completed",
			},
		})
	})
}
```

Note: `/readyz` and `/startupz` will be registered via the health.Checker in the router setup (requires ProvideRouter to accept `*health.Checker`). This will be wired during Wire generation. For now, `/livez` is added as a simple static route, and the full readyz/startupz integration will be finalized when the Wire graph compiles.

- [ ] **Step 5: Generate Wire code**

Run:
```bash
cd backend && go generate ./cmd/api/
```
Expected: generates `cmd/api/wire_gen.go`. If Wire errors occur (missing providers, circular deps), fix them iteratively.

- [ ] **Step 6: Verify API binary compiles**

Run:
```bash
cd backend && CGO_ENABLED=0 go build -tags embed -o bin/api ./cmd/api
```
Expected: compiles successfully. Note: this requires frontend dist/ to exist. For CI, also test without embed tag:
```bash
cd backend && CGO_ENABLED=0 go build -o bin/api ./cmd/api
```

- [ ] **Step 7: Commit**

```bash
git add backend/cmd/api/ backend/internal/health/ backend/internal/server/routes/common.go
git commit -m "feat(api): create cmd/api entry point with Wire DI

New API binary with its own Wire injector, cleanup function, and health
probes. Uses APIProviderSet which includes HTTP stack, request-path
async workers, and cache invalidation. No background maintenance loops."
```

---

### Task 4: Create `cmd/worker/` entry point

**Files:**
- Create: `backend/cmd/worker/main.go`
- Create: `backend/cmd/worker/wire.go`
- Copy: `backend/cmd/server/VERSION` → `backend/cmd/worker/VERSION`

- [ ] **Step 1: Copy VERSION file**

```bash
cp backend/cmd/server/VERSION backend/cmd/worker/VERSION
```

- [ ] **Step 2: Create `cmd/worker/main.go`**

Create `backend/cmd/worker/main.go`:

```go
package main

//go:generate go run github.com/google/wire/cmd/wire

import (
	"context"
	_ "embed"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

//go:embed VERSION
var embeddedVersion string

var (
	Version   = ""
	Commit    = "unknown"
	Date      = "unknown"
	BuildType = "source"
)

func init() {
	if strings.TrimSpace(Version) != "" {
		return
	}
	Version = strings.TrimSpace(embeddedVersion)
	if Version == "" {
		Version = "0.0.0-dev"
	}
}

func main() {
	logger.InitBootstrap()
	defer logger.Sync()

	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		log.Printf("Sub2API Worker %s (commit: %s, built: %s)\n", Version, Commit, Date)
		return
	}

	runWorker()
}

func runWorker() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if err := logger.Init(logger.OptionsFromConfig(cfg.Log)); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	app, err := initializeWorkerApplication()
	if err != nil {
		log.Fatalf("Failed to initialize worker application: %v", err)
	}
	defer app.Cleanup()

	// Start internal health HTTP server
	healthPort := "8081"
	if cfg.Worker.HealthPort != "" {
		healthPort = cfg.Worker.HealthPort
	}

	healthMux := http.NewServeMux()
	app.Health.RegisterOnMux(healthMux)
	healthServer := &http.Server{
		Addr:              net.JoinHostPort("0.0.0.0", healthPort),
		Handler:           healthMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Mark ready for startup probe
	app.Health.SetReady()

	go func() {
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Worker health server error: %v", err)
		}
	}()

	log.Printf("Worker started (health on :%s)", healthPort)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down worker...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := healthServer.Shutdown(ctx); err != nil {
		log.Printf("Worker health server forced to shutdown: %v", err)
	}

	log.Println("Worker exited")
}
```

- [ ] **Step 3: Add worker config field**

Add to the config struct in `backend/internal/config/config.go` (find the top-level Config struct):

```go
Worker struct {
	HealthPort string `mapstructure:"health_port"`
} `mapstructure:"worker"`
```

- [ ] **Step 4: Create `cmd/worker/wire.go`**

Create `backend/cmd/worker/wire.go`:

```go
//go:build wireinject
// +build wireinject

package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/health"
	appelotel "github.com/Wei-Shaw/sub2api/internal/pkg/otel"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
)

type WorkerApplication struct {
	Health  *health.Checker
	Cleanup func()
}

func initializeWorkerApplication() (*WorkerApplication, error) {
	wire.Build(
		config.ProviderSet,
		appelotel.ProviderSet,
		repository.ProviderSet,
		service.WorkerProviderSet,

		providePrivacyClientFactory,
		health.NewChecker,
		provideWorkerCleanup,

		wire.Struct(new(WorkerApplication), "*"),
	)
	return nil, nil
}

func providePrivacyClientFactory() service.PrivacyClientFactory {
	return repository.CreatePrivacyReqClient
}

func provideWorkerCleanup(
	entClient *ent.Client,
	rdb *redis.Client,
	otelProvider *appelotel.Provider,
	schedulerSnapshot *service.SchedulerSnapshotService,
	tokenRefresh *service.TokenRefreshService,
	accountExpiry *service.AccountExpiryService,
	subscriptionExpiry *service.SubscriptionExpiryService,
	usageCleanup *service.UsageCleanupService,
	idempotencyCleanup *service.IdempotencyCleanupService,
	pricing *service.PricingService,
	scheduledTestRunner *service.ScheduledTestRunnerService,
	backupSvc *service.BackupService,
	dashboardAgg *service.DashboardAggregationService,
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
			{"BackupService", func() error {
				if backupSvc != nil {
					backupSvc.Stop()
				}
				return nil
			}},
			{"DashboardAggregationService", func() error {
				if dashboardAgg != nil {
					dashboardAgg.Stop()
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

		var wg sync.WaitGroup
		for i := range parallelSteps {
			step := parallelSteps[i]
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := step.fn(); err != nil {
					log.Printf("[Worker Cleanup] %s failed: %v", step.name, err)
					return
				}
				log.Printf("[Worker Cleanup] %s succeeded", step.name)
			}()
		}
		wg.Wait()

		if otelProvider != nil {
			if err := otelProvider.Shutdown(ctx); err != nil {
				log.Printf("OTel provider shutdown error: %v", err)
			}
		}

		for _, step := range infraSteps {
			if err := step.fn(); err != nil {
				log.Printf("[Worker Cleanup] %s failed: %v", step.name, err)
				continue
			}
			log.Printf("[Worker Cleanup] %s succeeded", step.name)
		}

		select {
		case <-ctx.Done():
			log.Printf("[Worker Cleanup] Warning: cleanup timed out after 30 seconds")
		default:
			log.Printf("[Worker Cleanup] All cleanup steps completed")
		}
	}
}
```

- [ ] **Step 5: Generate Wire code**

Run:
```bash
cd backend && go generate ./cmd/worker/
```
Expected: generates `cmd/worker/wire_gen.go`. Fix any Wire errors iteratively.

- [ ] **Step 6: Verify worker binary compiles**

Run:
```bash
cd backend && CGO_ENABLED=0 go build -o bin/worker ./cmd/worker
```
Expected: compiles without errors. Worker does NOT need `-tags embed`.

- [ ] **Step 7: Commit**

```bash
git add backend/cmd/worker/ backend/internal/config/config.go
git commit -m "feat(worker): create cmd/worker entry point with Wire DI

New Worker binary with its own Wire injector, cleanup function, and
internal health server on port 8081. Uses WorkerProviderSet which
includes all background maintenance loops. No HTTP router or handlers."
```

---

### Task 5: Wire generation debugging and compilation verification

**Files:**
- Modify: `backend/internal/service/wire.go` (as needed for Wire errors)
- Modify: `backend/cmd/api/wire.go` (as needed)
- Modify: `backend/cmd/worker/wire.go` (as needed)

Wire generation is the most likely place for errors. This task is dedicated to making both binaries compile.

- [ ] **Step 1: Generate Wire for API**

Run:
```bash
cd backend && go generate ./cmd/api/
```

If Wire fails with "no provider for X", add the missing provider to `APIProviderSet` or `SharedProviderSet`. Common issues:
- Missing interface bindings (`wire.Bind`)
- Services that depend on worker-only services transitively
- Missing `provideServiceBuildInfo` (only needed by API, not worker)

- [ ] **Step 2: Generate Wire for Worker**

Run:
```bash
cd backend && go generate ./cmd/worker/
```

If Wire fails, the worker may need providers for shared services that it doesn't directly use but are required transitively. Add them to `WorkerProviderSet`.

- [ ] **Step 3: Build both binaries**

Run:
```bash
cd backend && CGO_ENABLED=0 go build -o bin/api ./cmd/api && CGO_ENABLED=0 go build -o bin/worker ./cmd/worker
```
Expected: both compile.

- [ ] **Step 4: Run existing tests**

Run:
```bash
cd backend && go test -tags=unit ./...
```
Expected: all existing tests pass. The old `cmd/server/` still compiles via the `ProviderSet = APIProviderSet` alias.

- [ ] **Step 5: Commit**

```bash
git add backend/
git commit -m "fix(wire): resolve Wire generation for api and worker binaries

Fix transitive dependency issues and missing providers discovered
during Wire code generation for the split binaries."
```

---

### Task 6: Delete `cmd/server/` and update Makefile

**Files:**
- Delete: `backend/cmd/server/` (entire directory)
- Modify: `backend/Makefile`
- Modify: `backend/internal/service/wire.go` (remove deprecated alias)

- [ ] **Step 1: Remove deprecated ProviderSet alias**

In `backend/internal/service/wire.go`, remove:
```go
// ProviderSet is DEPRECATED — use APIProviderSet or WorkerProviderSet.
var ProviderSet = APIProviderSet
```

- [ ] **Step 2: Delete cmd/server/**

```bash
rm -rf backend/cmd/server/
```

- [ ] **Step 3: Update Makefile**

Replace `backend/Makefile` content:

```makefile
.PHONY: build build-api build-worker build-bootstrap generate test test-unit test-integration test-e2e

VERSION ?= $(shell tr -d '\r\n' < ./cmd/api/VERSION)
LDFLAGS ?= -s -w -X main.Version=$(VERSION)

build: build-api build-worker

build-api:
	CGO_ENABLED=0 go build -tags embed -ldflags="$(LDFLAGS)" -trimpath -o bin/api ./cmd/api

build-worker:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o bin/worker ./cmd/worker

build-bootstrap:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o bin/bootstrap ./cmd/bootstrap

generate:
	go generate ./ent
	go generate ./cmd/api
	go generate ./cmd/worker

test:
	go test ./...
	golangci-lint run ./...

test-unit:
	go test -tags=unit ./...

test-integration:
	go test -tags=integration ./...

test-e2e:
	./scripts/e2e-test.sh

test-e2e-local:
	go test -tags=e2e -v -timeout=300s ./internal/integration/...
```

- [ ] **Step 4: Verify build**

Run:
```bash
cd backend && make build
```
Expected: produces `bin/api` and `bin/worker`.

- [ ] **Step 5: Verify tests**

Run:
```bash
cd backend && go test -tags=unit ./...
```
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add -A backend/cmd/server/ backend/Makefile backend/internal/service/wire.go
git commit -m "refactor: delete cmd/server, update Makefile for api+worker

Clean break from the monolithic server binary. Make build now produces
bin/api and bin/worker. make generate runs Wire for both binaries."
```

---

### Task 7: Docker images

**Files:**
- Create: `Dockerfile.api` (replaces `Dockerfile`)
- Create: `Dockerfile.worker`
- Create: `Dockerfile.goreleaser.api` (replaces `Dockerfile.goreleaser`)
- Create: `Dockerfile.goreleaser.worker`
- Delete: `Dockerfile` and `Dockerfile.goreleaser`

- [ ] **Step 1: Create `Dockerfile.api`**

Create `Dockerfile.api` at repo root (adapted from current `Dockerfile`, replacing `cmd/server` with `cmd/api`):

```dockerfile
# =============================================================================
# Sub2API API Server — Multi-Stage Dockerfile
# =============================================================================
ARG NODE_IMAGE=node:24-alpine
ARG GOLANG_IMAGE=golang:1.26.1-alpine
ARG ALPINE_IMAGE=alpine:3.21
ARG POSTGRES_IMAGE=postgres:18-alpine
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn

# Stage 1: Frontend Builder
FROM ${NODE_IMAGE} AS frontend-builder
WORKDIR /app/frontend
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm run build

# Stage 2: Backend Builder
FROM ${GOLANG_IMAGE} AS backend-builder
ARG VERSION=
ARG COMMIT=docker
ARG DATE
ARG GOPROXY
ARG GOSUMDB
ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}
RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend-builder /app/backend/internal/web/dist ./internal/web/dist
RUN VERSION_VALUE="${VERSION}" && \
    if [ -z "${VERSION_VALUE}" ]; then VERSION_VALUE="$(tr -d '\r\n' < ./cmd/api/VERSION)"; fi && \
    DATE_VALUE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" && \
    CGO_ENABLED=0 GOOS=linux go build \
    -tags embed \
    -ldflags="-s -w -X main.Version=${VERSION_VALUE} -X main.Commit=${COMMIT} -X main.Date=${DATE_VALUE} -X main.BuildType=release" \
    -trimpath \
    -o /app/sub2api-api \
    ./cmd/api

# Stage 3: PostgreSQL Client
FROM ${POSTGRES_IMAGE} AS pg-client

# Stage 4: Final Runtime Image
FROM ${ALPINE_IMAGE}
LABEL maintainer="Wccccc <github.com/wchen99998>"
LABEL description="Sub2API API Server - AI API Gateway Platform"
LABEL org.opencontainers.image.source="https://github.com/wchen99998/sub2api"
RUN apk add --no-cache ca-certificates tzdata libpq zstd-libs lz4-libs krb5-libs libldap libedit && rm -rf /var/cache/apk/*
COPY --from=pg-client /usr/local/bin/pg_dump /usr/local/bin/pg_dump
COPY --from=pg-client /usr/local/bin/psql /usr/local/bin/psql
COPY --from=pg-client /usr/local/lib/libpq.so.5* /usr/local/lib/
RUN addgroup -g 1000 sub2api && adduser -u 1000 -G sub2api -s /bin/sh -D sub2api
WORKDIR /app
COPY --from=backend-builder --chown=sub2api:sub2api /app/sub2api-api /app/sub2api-api
COPY --from=backend-builder --chown=sub2api:sub2api /app/backend/resources /app/resources
USER sub2api
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -q -T 5 -O /dev/null http://localhost:${SERVER_PORT:-8080}/readyz || exit 1
ENTRYPOINT ["/app/sub2api-api"]
```

- [ ] **Step 2: Create `Dockerfile.worker`**

Create `Dockerfile.worker` at repo root (similar to `Dockerfile.bootstrap` — minimal, no frontend):

```dockerfile
# =============================================================================
# Sub2API Worker — Multi-Stage Dockerfile
# =============================================================================
ARG GOLANG_IMAGE=golang:1.26.1-alpine
ARG ALPINE_IMAGE=alpine:3.21
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn

# Stage 1: Backend Builder
FROM ${GOLANG_IMAGE} AS backend-builder
ARG VERSION=
ARG COMMIT=docker
ARG DATE
ARG GOPROXY
ARG GOSUMDB
ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}
RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN VERSION_VALUE="${VERSION}" && \
    if [ -z "${VERSION_VALUE}" ]; then VERSION_VALUE="$(tr -d '\r\n' < ./cmd/worker/VERSION)"; fi && \
    DATE_VALUE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=${VERSION_VALUE} -X main.Commit=${COMMIT} -X main.Date=${DATE_VALUE} -X main.BuildType=release" \
    -trimpath \
    -o /app/sub2api-worker \
    ./cmd/worker

# Stage 2: Final Runtime Image
FROM ${ALPINE_IMAGE}
LABEL maintainer="Wccccc <github.com/wchen99998>"
LABEL description="Sub2API Worker - Background Maintenance Service"
LABEL org.opencontainers.image.source="https://github.com/wchen99998/sub2api"
RUN apk add --no-cache ca-certificates tzdata && rm -rf /var/cache/apk/*
RUN addgroup -g 1000 sub2api && adduser -u 1000 -G sub2api -s /bin/sh -D sub2api
WORKDIR /app
COPY --from=backend-builder --chown=sub2api:sub2api /app/sub2api-worker /app/sub2api-worker
USER sub2api
EXPOSE 8081
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -q -T 5 -O /dev/null http://localhost:8081/readyz || exit 1
ENTRYPOINT ["/app/sub2api-worker"]
```

- [ ] **Step 3: Create `Dockerfile.goreleaser.api`**

Create `Dockerfile.goreleaser.api` at repo root (replaces `Dockerfile.goreleaser`):

```dockerfile
# =============================================================================
# Sub2API API Server — GoReleaser Dockerfile
# =============================================================================
ARG ALPINE_IMAGE=alpine:3.21
ARG POSTGRES_IMAGE=postgres:18-alpine

FROM ${POSTGRES_IMAGE} AS pg-client

FROM ${ALPINE_IMAGE}
LABEL maintainer="Wccccc <github.com/wchen99998>"
LABEL description="Sub2API API Server - AI API Gateway Platform"
LABEL org.opencontainers.image.source="https://github.com/wchen99998/sub2api"
RUN apk add --no-cache ca-certificates tzdata libpq zstd-libs lz4-libs krb5-libs libldap libedit && rm -rf /var/cache/apk/*
COPY --from=pg-client /usr/local/bin/pg_dump /usr/local/bin/pg_dump
COPY --from=pg-client /usr/local/bin/psql /usr/local/bin/psql
COPY --from=pg-client /usr/local/lib/libpq.so.5* /usr/local/lib/
RUN addgroup -g 1000 sub2api && adduser -u 1000 -G sub2api -s /bin/sh -D sub2api
WORKDIR /app
COPY sub2api-api /app/sub2api-api
COPY backend/resources /app/resources
RUN chown -R sub2api:sub2api /app
USER sub2api
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -q -T 5 -O /dev/null http://localhost:${SERVER_PORT:-8080}/readyz || exit 1
ENTRYPOINT ["/app/sub2api-api"]
```

- [ ] **Step 4: Create `Dockerfile.goreleaser.worker`**

Create `Dockerfile.goreleaser.worker` at repo root:

```dockerfile
# =============================================================================
# Sub2API Worker — GoReleaser Dockerfile
# =============================================================================
ARG ALPINE_IMAGE=alpine:3.21

FROM ${ALPINE_IMAGE}
LABEL maintainer="Wccccc <github.com/wchen99998>"
LABEL description="Sub2API Worker - Background Maintenance Service"
LABEL org.opencontainers.image.source="https://github.com/wchen99998/sub2api"
RUN apk add --no-cache ca-certificates tzdata && rm -rf /var/cache/apk/*
RUN addgroup -g 1000 sub2api && adduser -u 1000 -G sub2api -s /bin/sh -D sub2api
WORKDIR /app
COPY sub2api-worker /app/sub2api-worker
RUN chown -R sub2api:sub2api /app
USER sub2api
EXPOSE 8081
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -q -T 5 -O /dev/null http://localhost:8081/readyz || exit 1
ENTRYPOINT ["/app/sub2api-worker"]
```

- [ ] **Step 5: Delete old Dockerfiles**

```bash
rm Dockerfile Dockerfile.goreleaser
```

- [ ] **Step 6: Commit**

```bash
git add Dockerfile.api Dockerfile.worker Dockerfile.goreleaser.api Dockerfile.goreleaser.worker
git rm Dockerfile Dockerfile.goreleaser
git commit -m "refactor(docker): replace server image with api and worker images

Dockerfile.api includes frontend build and pg_dump/psql (same as old
Dockerfile but builds cmd/api). Dockerfile.worker is minimal Alpine
(similar to bootstrap). GoReleaser variants match the pattern."
```

---

### Task 8: Update GoReleaser config

**Files:**
- Modify: `.goreleaser.yaml`

- [ ] **Step 1: Update builds, dockers, and manifests**

Replace the `sub2api` build with `sub2api-api` and `sub2api-worker`. Replace `server-*` docker IDs with `api-*` and add `worker-*`. Replace `server` manifests with `api` and add `worker` manifests.

In `.goreleaser.yaml`, update the `builds` section:

```yaml
builds:
  - id: sub2api-api
    dir: backend
    main: ./cmd/api
    binary: sub2api-api
    flags:
      - -tags=embed
    env:
      - CGO_ENABLED=0
    goos:
      - linux
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w
      - -X main.Commit={{.Commit}}
      - -X main.Date={{.Date}}
      - -X main.BuildType=release

  - id: sub2api-worker
    dir: backend
    main: ./cmd/worker
    binary: sub2api-worker
    env:
      - CGO_ENABLED=0
    goos:
      - linux
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w
      - -X main.Commit={{.Commit}}
      - -X main.Date={{.Date}}
      - -X main.BuildType=release

  - id: sub2api-bootstrap
    dir: backend
    main: ./cmd/bootstrap
    binary: sub2api-bootstrap
    env:
      - CGO_ENABLED=0
    goos:
      - linux
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w
```

Update the `dockers` section — replace `server-amd64`/`server-arm64` with `api-amd64`/`api-arm64` and add `worker-amd64`/`worker-arm64`:

```yaml
dockers:
  - id: api-amd64
    ids: [sub2api-api]
    goos: linux
    goarch: amd64
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/api:{{ .Version }}-amd64"
    dockerfile: Dockerfile.goreleaser.api
    extra_files:
      - backend/resources
    build_flag_templates:
      - "--label=org.opencontainers.image.version={{ .Version }}"
      - "--label=org.opencontainers.image.revision={{ .Commit }}"
      - "--label=org.opencontainers.image.source=https://github.com/{{ .Env.GITHUB_REPO_OWNER }}/{{ .Env.GITHUB_REPO_NAME }}"

  - id: api-arm64
    ids: [sub2api-api]
    goos: linux
    goarch: arm64
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/api:{{ .Version }}-arm64"
    dockerfile: Dockerfile.goreleaser.api
    extra_files:
      - backend/resources
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--label=org.opencontainers.image.version={{ .Version }}"
      - "--label=org.opencontainers.image.revision={{ .Commit }}"
      - "--label=org.opencontainers.image.source=https://github.com/{{ .Env.GITHUB_REPO_OWNER }}/{{ .Env.GITHUB_REPO_NAME }}"

  - id: worker-amd64
    ids: [sub2api-worker]
    goos: linux
    goarch: amd64
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/worker:{{ .Version }}-amd64"
    dockerfile: Dockerfile.goreleaser.worker
    build_flag_templates:
      - "--label=org.opencontainers.image.version={{ .Version }}"
      - "--label=org.opencontainers.image.revision={{ .Commit }}"
      - "--label=org.opencontainers.image.source=https://github.com/{{ .Env.GITHUB_REPO_OWNER }}/{{ .Env.GITHUB_REPO_NAME }}"

  - id: worker-arm64
    ids: [sub2api-worker]
    goos: linux
    goarch: arm64
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/worker:{{ .Version }}-arm64"
    dockerfile: Dockerfile.goreleaser.worker
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--label=org.opencontainers.image.version={{ .Version }}"
      - "--label=org.opencontainers.image.revision={{ .Commit }}"
      - "--label=org.opencontainers.image.source=https://github.com/{{ .Env.GITHUB_REPO_OWNER }}/{{ .Env.GITHUB_REPO_NAME }}"

  # Bootstrap unchanged
  - id: bootstrap-amd64
    ids: [sub2api-bootstrap]
    goos: linux
    goarch: amd64
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-amd64"
    dockerfile: Dockerfile.goreleaser.bootstrap
    build_flag_templates:
      - "--label=org.opencontainers.image.version={{ .Version }}"
      - "--label=org.opencontainers.image.revision={{ .Commit }}"
      - "--label=org.opencontainers.image.source=https://github.com/{{ .Env.GITHUB_REPO_OWNER }}/{{ .Env.GITHUB_REPO_NAME }}"

  - id: bootstrap-arm64
    ids: [sub2api-bootstrap]
    goos: linux
    goarch: arm64
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-arm64"
    dockerfile: Dockerfile.goreleaser.bootstrap
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--label=org.opencontainers.image.version={{ .Version }}"
      - "--label=org.opencontainers.image.revision={{ .Commit }}"
      - "--label=org.opencontainers.image.source=https://github.com/{{ .Env.GITHUB_REPO_OWNER }}/{{ .Env.GITHUB_REPO_NAME }}"
```

Update `docker_manifests` — replace `server` with `api` and add `worker`:

```yaml
docker_manifests:
  # API manifests
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/api:{{ .Version }}"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/api:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/api:{{ .Version }}-arm64"
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/api:latest"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/api:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/api:{{ .Version }}-arm64"
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/api:{{ .Major }}.{{ .Minor }}"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/api:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/api:{{ .Version }}-arm64"
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/api:{{ .Major }}"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/api:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/api:{{ .Version }}-arm64"
  # Worker manifests
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/worker:{{ .Version }}"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/worker:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/worker:{{ .Version }}-arm64"
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/worker:latest"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/worker:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/worker:{{ .Version }}-arm64"
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/worker:{{ .Major }}.{{ .Minor }}"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/worker:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/worker:{{ .Version }}-arm64"
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/worker:{{ .Major }}"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/worker:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/worker:{{ .Version }}-arm64"
  # Bootstrap manifests (unchanged)
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-arm64"
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:latest"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-arm64"
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Major }}.{{ .Minor }}"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-arm64"
  - name_template: "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Major }}"
    image_templates:
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-amd64"
      - "ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/bootstrap:{{ .Version }}-arm64"
```

Also update the `release.footer` section to reference the new image:
```yaml
    docker pull ghcr.io/{{ .Env.GITHUB_REPO_OWNER_LOWER }}/{{ .Env.GITHUB_REPO_NAME }}/api:{{ .Version }}
```

- [ ] **Step 2: Commit**

```bash
git add .goreleaser.yaml
git commit -m "refactor(release): update GoReleaser for api+worker images

Replace server build/docker/manifest with api and worker. Three
binaries (api, worker, bootstrap), three images, multi-arch manifests."
```

---

### Task 9: Helm chart split

**Files:**
- Create: `deploy/helm/sub2api/templates/api-deployment.yaml`
- Create: `deploy/helm/sub2api/templates/api-service.yaml`
- Create: `deploy/helm/sub2api/templates/api-ingress.yaml`
- Create: `deploy/helm/sub2api/templates/api-hpa.yaml`
- Create: `deploy/helm/sub2api/templates/api-pdb.yaml`
- Create: `deploy/helm/sub2api/templates/worker-deployment.yaml`
- Delete: `deploy/helm/sub2api/templates/deployment.yaml`
- Delete: `deploy/helm/sub2api/templates/service.yaml`
- Delete: `deploy/helm/sub2api/templates/ingress.yaml`
- Modify: `deploy/helm/sub2api/templates/_helpers.tpl`
- Modify: `deploy/helm/sub2api/templates/servicemonitor.yaml`
- Modify: `deploy/helm/sub2api/values.yaml`
- Modify: `deploy/helm/sub2api/values-production.yaml`

- [ ] **Step 1: Update `_helpers.tpl` with component helpers**

Add to `deploy/helm/sub2api/templates/_helpers.tpl`:

```yaml
{{/*
API component labels.
*/}}
{{- define "sub2api.api.selectorLabels" -}}
{{ include "sub2api.selectorLabels" . }}
app.kubernetes.io/component: api
{{- end }}

{{/*
Worker component labels.
*/}}
{{- define "sub2api.worker.selectorLabels" -}}
{{ include "sub2api.selectorLabels" . }}
app.kubernetes.io/component: worker
{{- end }}
```

- [ ] **Step 2: Create `api-deployment.yaml`**

Create `deploy/helm/sub2api/templates/api-deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "sub2api.fullname" . }}-api
  labels:
    {{- include "sub2api.labels" . | nindent 4 }}
    app.kubernetes.io/component: api
spec:
  {{- if not .Values.api.autoscaling.enabled }}
  replicas: {{ .Values.api.replicaCount }}
  {{- end }}
  selector:
    matchLabels:
      {{- include "sub2api.api.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      annotations:
        checksum/configmap: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
        checksum/secret: {{ include (print $.Template.BasePath "/secret.yaml") . | sha256sum }}
      labels:
        {{- include "sub2api.labels" . | nindent 8 }}
        app.kubernetes.io/component: api
    spec:
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      serviceAccountName: {{ include "sub2api.serviceAccountName" . }}
      securityContext:
        fsGroup: 1000
      terminationGracePeriodSeconds: {{ .Values.api.terminationGracePeriodSeconds | default 5 }}
      containers:
        - name: api
          image: "{{ .Values.image.api.repository }}:{{ .Values.image.api.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.api.pullPolicy }}
          ports:
            - name: http
              containerPort: 8080
              protocol: TCP
            {{- if ((.Values.observability).enabled) }}
            - name: metrics
              containerPort: {{ .Values.observability.otel.metricsPort }}
              protocol: TCP
            {{- end }}
          envFrom:
            - configMapRef:
                name: {{ include "sub2api.fullname" . }}
            - secretRef:
                name: {{ include "sub2api.secretName" . }}
          startupProbe:
            httpGet:
              path: {{ .Values.api.probes.startup.path | default "/startupz" }}
              port: http
            failureThreshold: {{ .Values.api.probes.startup.failureThreshold | default 30 }}
            periodSeconds: {{ .Values.api.probes.startup.periodSeconds | default 2 }}
          livenessProbe:
            httpGet:
              path: {{ .Values.api.probes.liveness.path | default "/livez" }}
              port: http
            periodSeconds: {{ .Values.api.probes.liveness.periodSeconds | default 30 }}
            timeoutSeconds: 10
          readinessProbe:
            httpGet:
              path: {{ .Values.api.probes.readiness.path | default "/readyz" }}
              port: http
            periodSeconds: {{ .Values.api.probes.readiness.periodSeconds | default 10 }}
            timeoutSeconds: 5
          resources:
            {{- toYaml .Values.api.resources | nindent 12 }}
          securityContext:
            runAsNonRoot: true
            runAsUser: 1000
      {{- with .Values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
```

- [ ] **Step 3: Create `api-service.yaml`**

Create `deploy/helm/sub2api/templates/api-service.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: {{ include "sub2api.fullname" . }}-api
  labels:
    {{- include "sub2api.labels" . | nindent 4 }}
    app.kubernetes.io/component: api
spec:
  type: {{ .Values.service.type }}
  ports:
    - port: {{ .Values.service.port }}
      targetPort: http
      protocol: TCP
      name: http
  {{- if ((.Values.observability).enabled) }}
    - port: {{ .Values.observability.otel.metricsPort }}
      targetPort: metrics
      protocol: TCP
      name: metrics
  {{- end }}
  selector:
    {{- include "sub2api.api.selectorLabels" . | nindent 4 }}
```

- [ ] **Step 4: Create `api-ingress.yaml`**

Create `deploy/helm/sub2api/templates/api-ingress.yaml`:

```yaml
{{- if .Values.ingress.enabled }}
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ include "sub2api.fullname" . }}-api
  labels:
    {{- include "sub2api.labels" . | nindent 4 }}
    app.kubernetes.io/component: api
  annotations:
    external-dns.alpha.kubernetes.io/hostname: >-
      {{ .Values.ingress.host }}{{ range .Values.ingress.extraHosts }},{{ .host }}{{ end }}
    external-dns.alpha.kubernetes.io/cloudflare-proxied: {{ .Values.ingress.cloudflareProxied | quote }}
    {{- with .Values.ingress.annotations }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
spec:
  {{- if .Values.ingress.className }}
  ingressClassName: {{ .Values.ingress.className | quote }}
  {{- end }}
  {{- if .Values.ingress.tls.enabled }}
  tls:
    - hosts:
        - {{ .Values.ingress.host | quote }}
        {{- range .Values.ingress.extraHosts }}
        - {{ .host | quote }}
        {{- end }}
      secretName: {{ default (printf "%s-tls" (include "sub2api.fullname" .)) .Values.ingress.tls.secretName }}
  {{- end }}
  rules:
    - host: {{ .Values.ingress.host | quote }}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: {{ include "sub2api.fullname" . }}-api
                port:
                  name: http
    {{- range .Values.ingress.extraHosts }}
    - host: {{ .host | quote }}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: {{ include "sub2api.fullname" $ }}-api
                port:
                  name: http
    {{- end }}
{{- end }}
```

- [ ] **Step 5: Create `api-hpa.yaml`**

Create `deploy/helm/sub2api/templates/api-hpa.yaml`:

```yaml
{{- if .Values.api.autoscaling.enabled }}
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ include "sub2api.fullname" . }}-api
  labels:
    {{- include "sub2api.labels" . | nindent 4 }}
    app.kubernetes.io/component: api
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {{ include "sub2api.fullname" . }}-api
  minReplicas: {{ .Values.api.autoscaling.minReplicas }}
  maxReplicas: {{ .Values.api.autoscaling.maxReplicas }}
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: {{ .Values.api.autoscaling.targetCPUUtilization }}
{{- end }}
```

- [ ] **Step 6: Create `api-pdb.yaml`**

Create `deploy/helm/sub2api/templates/api-pdb.yaml`:

```yaml
{{- if .Values.api.pdb.enabled }}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "sub2api.fullname" . }}-api
  labels:
    {{- include "sub2api.labels" . | nindent 4 }}
    app.kubernetes.io/component: api
spec:
  minAvailable: {{ .Values.api.pdb.minAvailable }}
  selector:
    matchLabels:
      {{- include "sub2api.api.selectorLabels" . | nindent 6 }}
{{- end }}
```

- [ ] **Step 7: Create `worker-deployment.yaml`**

Create `deploy/helm/sub2api/templates/worker-deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "sub2api.fullname" . }}-worker
  labels:
    {{- include "sub2api.labels" . | nindent 4 }}
    app.kubernetes.io/component: worker
spec:
  replicas: {{ .Values.worker.replicaCount }}
  selector:
    matchLabels:
      {{- include "sub2api.worker.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      annotations:
        checksum/configmap: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
        checksum/secret: {{ include (print $.Template.BasePath "/secret.yaml") . | sha256sum }}
      labels:
        {{- include "sub2api.labels" . | nindent 8 }}
        app.kubernetes.io/component: worker
    spec:
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      serviceAccountName: {{ include "sub2api.serviceAccountName" . }}
      securityContext:
        fsGroup: 1000
      terminationGracePeriodSeconds: {{ .Values.worker.terminationGracePeriodSeconds | default 30 }}
      containers:
        - name: worker
          image: "{{ .Values.image.worker.repository }}:{{ .Values.image.worker.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.worker.pullPolicy }}
          ports:
            - name: health
              containerPort: 8081
              protocol: TCP
          envFrom:
            - configMapRef:
                name: {{ include "sub2api.fullname" . }}
            - secretRef:
                name: {{ include "sub2api.secretName" . }}
          startupProbe:
            httpGet:
              path: {{ .Values.worker.probes.startup.path | default "/startupz" }}
              port: health
            failureThreshold: {{ .Values.worker.probes.startup.failureThreshold | default 30 }}
            periodSeconds: {{ .Values.worker.probes.startup.periodSeconds | default 2 }}
          livenessProbe:
            httpGet:
              path: {{ .Values.worker.probes.liveness.path | default "/livez" }}
              port: health
            periodSeconds: {{ .Values.worker.probes.liveness.periodSeconds | default 30 }}
            timeoutSeconds: 10
          readinessProbe:
            httpGet:
              path: {{ .Values.worker.probes.readiness.path | default "/readyz" }}
              port: health
            periodSeconds: {{ .Values.worker.probes.readiness.periodSeconds | default 10 }}
            timeoutSeconds: 5
          resources:
            {{- toYaml .Values.worker.resources | nindent 12 }}
          securityContext:
            runAsNonRoot: true
            runAsUser: 1000
      {{- with .Values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
```

- [ ] **Step 8: Delete old templates**

```bash
rm deploy/helm/sub2api/templates/deployment.yaml
rm deploy/helm/sub2api/templates/service.yaml
rm deploy/helm/sub2api/templates/ingress.yaml
```

- [ ] **Step 9: Update `servicemonitor.yaml` selector**

Update the selector in `deploy/helm/sub2api/templates/servicemonitor.yaml` to target the API service:

Replace:
```yaml
  selector:
    matchLabels:
      {{- include "sub2api.selectorLabels" . | nindent 6 }}
```
With:
```yaml
  selector:
    matchLabels:
      {{- include "sub2api.api.selectorLabels" . | nindent 6 }}
```

- [ ] **Step 10: Update `values.yaml`**

Replace `replicaCount`, `image`, and `resources` sections with the split structure. Replace the old top-level keys:

```yaml
# -- API Configuration
api:
  replicaCount: 1
  terminationGracePeriodSeconds: 5
  resources:
    requests:
      cpu: 250m
      memory: 512Mi
    limits:
      cpu: "1"
      memory: 1Gi
  autoscaling:
    enabled: false
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilization: 70
  pdb:
    enabled: false
    minAvailable: 1
  probes:
    startup:
      path: /startupz
      failureThreshold: 30
      periodSeconds: 2
    liveness:
      path: /livez
      periodSeconds: 30
    readiness:
      path: /readyz
      periodSeconds: 10

# -- Worker Configuration
worker:
  replicaCount: 1
  terminationGracePeriodSeconds: 30
  resources:
    requests:
      cpu: 100m
      memory: 256Mi
    limits:
      cpu: "1"
      memory: 1Gi
  probes:
    startup:
      path: /startupz
      failureThreshold: 30
      periodSeconds: 2
    liveness:
      path: /livez
      periodSeconds: 30
    readiness:
      path: /readyz
      periodSeconds: 10

image:
  api:
    repository: ghcr.io/wchen99998/sub2api/api
    tag: ""
    pullPolicy: IfNotPresent
  worker:
    repository: ghcr.io/wchen99998/sub2api/worker
    tag: ""
    pullPolicy: IfNotPresent
  bootstrap:
    repository: ghcr.io/wchen99998/sub2api/bootstrap
    tag: ""
    pullPolicy: IfNotPresent
```

Remove the old top-level `replicaCount: 1` and `resources:` block.

- [ ] **Step 11: Update `values-production.yaml`**

Replace:
```yaml
replicaCount: 2

resources:
  requests:
    cpu: 250m
    memory: 512Mi
  limits:
    cpu: "2"
    memory: 2Gi
```

With:
```yaml
api:
  replicaCount: 2
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilization: 70
  pdb:
    enabled: true
    minAvailable: 1
  resources:
    requests:
      cpu: 250m
      memory: 512Mi
    limits:
      cpu: "2"
      memory: 2Gi

worker:
  replicaCount: 1
  resources:
    requests:
      cpu: 100m
      memory: 256Mi
    limits:
      cpu: "1"
      memory: 1Gi
```

- [ ] **Step 12: Validate Helm template rendering**

Run:
```bash
helm template test deploy/helm/sub2api/ 2>&1 | head -200
```
Expected: renders api-deployment, api-service, api-ingress, worker-deployment, bootstrap-job, configmap, secret, serviceaccount without errors.

- [ ] **Step 13: Commit**

```bash
git add deploy/helm/sub2api/
git commit -m "refactor(helm): split into api and worker deployments

Replace single deployment with api-deployment (Service, Ingress, HPA,
PDB) and worker-deployment (no Service/Ingress). Worker singleton by
default, API scalable with optional HPA. New probe endpoints
(/livez, /readyz, /startupz) for both roles."
```

---

### Task 10: Update CI workflow

**Files:**
- Modify: `.github/workflows/release.yml` (if it references `cmd/server` or the old image name)
- Modify: `.github/workflows/backend-ci.yml` (if it builds `cmd/server`)

- [ ] **Step 1: Check CI for cmd/server references and update**

Search for any `cmd/server` references in `.github/workflows/` and update them to `cmd/api` / `cmd/worker`. Also check for image name references to `server` that should become `api`.

Run:
```bash
grep -r "cmd/server" .github/workflows/ || echo "No references found"
grep -r "server:" .github/workflows/ || echo "No references found"
```

Update any found references.

- [ ] **Step 2: Update deploy scripts if any**

Check `deploy/` and `DEPLOY.md` for references to the old server image and update them.

Run:
```bash
grep -r "sub2api/server" deploy/ DEPLOY.md || echo "No references found"
```

- [ ] **Step 3: Commit**

```bash
git add .github/ deploy/ DEPLOY.md
git commit -m "fix(ci): update workflows and deploy docs for api+worker split"
```

---

### Task 11: Final verification

**Files:** None (verification only)

- [ ] **Step 1: Build both binaries**

```bash
cd backend && make build
```
Expected: `bin/api` and `bin/worker` both produced.

- [ ] **Step 2: Run unit tests**

```bash
cd backend && go test -tags=unit ./...
```
Expected: all pass.

- [ ] **Step 3: Run linter**

```bash
cd backend && golangci-lint run ./...
```
Expected: clean.

- [ ] **Step 4: Validate Helm chart**

```bash
helm lint deploy/helm/sub2api/
helm template test deploy/helm/sub2api/ > /dev/null
```
Expected: no errors.

- [ ] **Step 5: Verify Wire generation is deterministic**

```bash
cd backend && make generate
git diff --exit-code
```
Expected: no changes (generated code matches committed code).
