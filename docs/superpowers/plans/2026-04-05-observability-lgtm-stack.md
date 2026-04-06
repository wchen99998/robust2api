# LGTM Observability Stack Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add OpenTelemetry instrumentation to the Go backend and deploy the full LGTM stack (Loki, Grafana, Tempo, Prometheus) via Helm into the DOKS cluster, alongside the existing custom observability system.

**Architecture:** OTel SDK in the Go app exports metrics (OTLP + Prometheus `/metrics`) and traces (OTLP) to Grafana Alloy, which forwards to Prometheus, Tempo, and Loki. A dedicated internal HTTP server on `:9090` exposes metrics. Zap logs get trace ID injection for correlation. Pre-provisioned Grafana dashboards provide visibility.

**Tech Stack:** Go 1.26, OpenTelemetry Go SDK, Gin, Zap, Wire DI, Helm 3, kube-prometheus-stack, Grafana Tempo, Grafana Loki, Grafana Alloy

---

## File Structure

### New Files (Backend)
- `backend/internal/pkg/otel/otel.go` — OTel SDK bootstrap (TracerProvider, MeterProvider, shutdown)
- `backend/internal/pkg/otel/metrics.go` — Business metric definitions (counters, histograms, gauges)
- `backend/internal/pkg/otel/metrics_server.go` — Internal HTTP server on `:9090` for `/metrics`
- `backend/internal/pkg/otel/wire.go` — Wire provider set
- `backend/internal/pkg/otel/otel_test.go` — Unit tests for bootstrap
- `backend/internal/pkg/otel/metrics_test.go` — Unit tests for metric recording
- `backend/internal/server/middleware/otel.go` — OTel Gin middleware + X-Trace-Id header middleware
- `backend/internal/pkg/logger/tracectx.go` — Trace ID extraction from OTel context for Zap

### Modified Files (Backend)
- `backend/internal/config/config.go` — Add `OtelConfig` struct and defaults
- `backend/cmd/server/wire.go` — Add OTel provider set, cleanup
- `backend/cmd/server/wire_gen.go` — Regenerated
- `backend/internal/server/router.go` — Add otelgin + trace ID middleware
- `backend/internal/handler/gateway_handler.go` — Add tracing spans in Messages/CountTokens
- `backend/internal/service/gateway_service.go` — Add tracing spans in Forward, upstream request, response handling
- `backend/go.mod` / `backend/go.sum` — New OTel dependencies

### New Files (Helm)
- `deploy/helm/monitoring/Chart.yaml` — Monitoring umbrella chart
- `deploy/helm/monitoring/values.yaml` — Default values for LGTM stack
- `deploy/helm/monitoring/templates/namespace.yaml` — monitoring namespace
- `deploy/helm/monitoring/templates/alloy-config.yaml` — Alloy pipeline config
- `deploy/helm/monitoring/templates/grafana-dashboards.yaml` — ConfigMap with dashboard JSON
- `deploy/helm/monitoring/templates/grafana-datasources.yaml` — Provisioned data sources
- `deploy/helm/sub2api/templates/servicemonitor.yaml` — ServiceMonitor for metrics scraping

### Modified Files (Helm)
- `deploy/helm/sub2api/values.yaml` — Add observability section
- `deploy/helm/sub2api/templates/deployment.yaml` — Add metrics container port
- `deploy/helm/sub2api/templates/service.yaml` — Add metrics service port
- `deploy/helm/sub2api/templates/configmap.yaml` — Add OTel env vars

---

## Task 1: OTel Configuration

**Files:**
- Modify: `backend/internal/config/config.go:55-86` (Config struct), `backend/internal/config/config.go:1125` (setDefaults)

- [ ] **Step 1: Add OtelConfig struct to config.go**

Add after the `IdempotencyConfig` definition (find the last config struct definition before the `Config` struct):

```go
type OtelConfig struct {
	Enabled         bool    `mapstructure:"enabled"`
	ServiceName     string  `mapstructure:"service_name"`
	Endpoint        string  `mapstructure:"endpoint"`
	TraceSampleRate float64 `mapstructure:"trace_sample_rate"`
	MetricsPort     int     `mapstructure:"metrics_port"`
}
```

- [ ] **Step 2: Add Otel field to Config struct**

Add to the `Config` struct at `config.go:85` (after the Idempotency field):

```go
Otel OtelConfig `mapstructure:"otel"`
```

- [ ] **Step 3: Add defaults in setDefaults()**

Add after the existing defaults block (e.g., after the idempotency defaults):

```go
// OpenTelemetry
viper.SetDefault("otel.enabled", false)
viper.SetDefault("otel.service_name", "sub2api")
viper.SetDefault("otel.endpoint", "http://alloy.monitoring.svc:4318")
viper.SetDefault("otel.trace_sample_rate", 0.1)
viper.SetDefault("otel.metrics_port", 9090)
```

- [ ] **Step 4: Verify compilation**

Run: `cd backend && go build ./...`
Expected: Build succeeds with no errors.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/config/config.go
git commit -m "feat(otel): add OtelConfig to application configuration"
```

---

## Task 2: OTel SDK Bootstrap Package

**Files:**
- Create: `backend/internal/pkg/otel/otel.go`
- Create: `backend/internal/pkg/otel/otel_test.go`

- [ ] **Step 1: Install OTel dependencies**

```bash
cd backend && go get \
  go.opentelemetry.io/otel \
  go.opentelemetry.io/otel/sdk \
  go.opentelemetry.io/otel/sdk/metric \
  go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp \
  go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp \
  go.opentelemetry.io/otel/exporters/prometheus \
  go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin \
  go.opentelemetry.io/contrib/instrumentation/runtime \
  github.com/prometheus/client_golang/prometheus/promhttp
```

- [ ] **Step 2: Write the failing test for OTel bootstrap**

Create `backend/internal/pkg/otel/otel_test.go`:

```go
package otel

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestInit_Disabled(t *testing.T) {
	cfg := &config.OtelConfig{Enabled: false}
	provider, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if provider == nil {
		t.Fatal("Init() returned nil provider")
	}
	// Shutdown should be a no-op when disabled
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestInit_Enabled_NoEndpoint(t *testing.T) {
	// With enabled=true but no real endpoint, Init should still succeed
	// (exporters connect lazily)
	cfg := &config.OtelConfig{
		Enabled:         true,
		ServiceName:     "test-service",
		Endpoint:        "http://localhost:4318",
		TraceSampleRate: 1.0,
		MetricsPort:     0, // 0 means pick random port
	}
	provider, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if provider == nil {
		t.Fatal("Init() returned nil provider")
	}
	if provider.TracerProvider() == nil {
		t.Fatal("TracerProvider() returned nil")
	}
	if provider.MeterProvider() == nil {
		t.Fatal("MeterProvider() returned nil")
	}
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd backend && go test ./internal/pkg/otel/ -v -count=1`
Expected: FAIL — package does not exist yet.

- [ ] **Step 4: Implement OTel bootstrap**

Create `backend/internal/pkg/otel/otel.go`:

```go
package otel

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Provider holds the initialized OTel providers and exposes a Shutdown method.
type Provider struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	promExporter   *promexporter.Exporter
	enabled        bool
}

// TracerProvider returns the configured tracer provider, or nil if disabled.
func (p *Provider) TracerProvider() *sdktrace.TracerProvider {
	return p.tracerProvider
}

// MeterProvider returns the configured meter provider, or nil if disabled.
func (p *Provider) MeterProvider() *sdkmetric.MeterProvider {
	return p.meterProvider
}

// PrometheusExporter returns the Prometheus exporter for the metrics HTTP handler.
func (p *Provider) PrometheusExporter() *promexporter.Exporter {
	return p.promExporter
}

// Shutdown flushes and shuts down all providers.
func (p *Provider) Shutdown(ctx context.Context) error {
	if !p.enabled {
		return nil
	}
	var errs []error
	if p.tracerProvider != nil {
		if err := p.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("tracer shutdown: %w", err))
		}
	}
	if p.meterProvider != nil {
		if err := p.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("meter shutdown: %w", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("otel shutdown errors: %v", errs)
	}
	return nil
}

// Init initializes OTel tracing and metrics providers.
// If cfg.Enabled is false, returns a no-op Provider.
func Init(ctx context.Context, cfg *config.OtelConfig) (*Provider, error) {
	if !cfg.Enabled {
		return &Provider{enabled: false}, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	// --- Tracer ---
	traceExporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(cfg.Endpoint+"/v1/traces"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating trace exporter: %w", err)
	}

	sampler := sdktrace.ParentBased(
		sdktrace.TraceIDRatioBased(cfg.TraceSampleRate),
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tp)

	// --- Meter ---
	metricExporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpointURL(cfg.Endpoint+"/v1/metrics"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating metric exporter: %w", err)
	}

	promExp, err := promexporter.New()
	if err != nil {
		return nil, fmt.Errorf("creating prometheus exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithReader(promExp),
	)
	otel.SetMeterProvider(mp)

	return &Provider{
		tracerProvider: tp,
		meterProvider:  mp,
		promExporter:   promExp,
		enabled:        true,
	}, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd backend && go test ./internal/pkg/otel/ -v -count=1`
Expected: PASS for both TestInit_Disabled and TestInit_Enabled_NoEndpoint.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/pkg/otel/otel.go backend/internal/pkg/otel/otel_test.go backend/go.mod backend/go.sum
git commit -m "feat(otel): add OTel SDK bootstrap package with tracer and meter providers"
```

---

## Task 3: Internal Metrics Server

**Files:**
- Create: `backend/internal/pkg/otel/metrics_server.go`

- [ ] **Step 1: Implement the internal metrics server**

Create `backend/internal/pkg/otel/metrics_server.go`:

```go
package otel

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
)

// MetricsServer serves Prometheus metrics and pprof on an internal port.
type MetricsServer struct {
	server *http.Server
}

// NewMetricsServer creates a metrics server bound to the given port.
// Pass port=0 to auto-select an available port.
func NewMetricsServer(port int, promExp *promexporter.Exporter) *MetricsServer {
	mux := http.NewServeMux()

	// Prometheus metrics endpoint
	if promExp != nil {
		mux.Handle("/metrics", promhttp.Handler())
	}

	// pprof endpoints
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// Health check for the metrics server itself
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return &MetricsServer{
		server: &http.Server{
			Addr:              fmt.Sprintf(":%d", port),
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

// Start begins listening. It blocks until the server stops.
func (s *MetricsServer) Start() error {
	ln, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return fmt.Errorf("metrics server listen: %w", err)
	}
	log.Printf("Metrics server listening on %s", ln.Addr().String())
	return s.server.Serve(ln)
}

// Shutdown gracefully stops the metrics server.
func (s *MetricsServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd backend && go build ./internal/pkg/otel/...`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/pkg/otel/metrics_server.go
git commit -m "feat(otel): add internal metrics server for Prometheus and pprof"
```

---

## Task 4: Business Metrics Definitions

**Files:**
- Create: `backend/internal/pkg/otel/metrics.go`
- Create: `backend/internal/pkg/otel/metrics_test.go`

- [ ] **Step 1: Write the failing test for metric recording**

Create `backend/internal/pkg/otel/metrics_test.go`:

```go
package otel

import (
	"context"
	"testing"
)

func TestMetrics_RecordRequest(t *testing.T) {
	m, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics() error = %v", err)
	}
	// Should not panic when recording metrics
	m.RecordRequest(context.Background(), "POST", "/v1/messages", 200, "anthropic")
	m.RecordDuration(context.Background(), 0.150, "POST", "/v1/messages", 200, "anthropic")
	m.RecordTTFT(context.Background(), 0.050, "anthropic", "claude-sonnet-4-20250514")
	m.RecordTokens(context.Background(), 100, "input", "anthropic", "claude-sonnet-4-20250514")
	m.RecordTokens(context.Background(), 200, "output", "anthropic", "claude-sonnet-4-20250514")
	m.RecordUpstreamError(context.Background(), "anthropic", "429")
	m.RecordAccountFailover(context.Background(), "anthropic")
	m.RecordRateLimitRejection(context.Background(), "api_key")
	m.SetConcurrencyQueueDepth(context.Background(), 42)
	m.SetUpstreamAccountsActive(context.Background(), 10, "anthropic")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/pkg/otel/ -run TestMetrics -v -count=1`
Expected: FAIL — NewMetrics not defined.

- [ ] **Step 3: Implement business metrics**

Create `backend/internal/pkg/otel/metrics.go`:

```go
package otel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const meterName = "github.com/Wei-Shaw/sub2api"

// Metrics holds all application-level OTel metric instruments.
type Metrics struct {
	httpRequestsTotal       metric.Int64Counter
	httpRequestDuration     metric.Float64Histogram
	httpRequestTTFT         metric.Float64Histogram
	tokensTotal             metric.Int64Counter
	upstreamErrorsTotal     metric.Int64Counter
	accountFailoversTotal   metric.Int64Counter
	rateLimitRejectionsTotal metric.Int64Counter
	concurrencyQueueDepth   metric.Int64Gauge
	upstreamAccountsActive  metric.Int64Gauge
}

// NewMetrics creates and registers all application metric instruments.
func NewMetrics() (*Metrics, error) {
	meter := otel.Meter(meterName)
	m := &Metrics{}
	var err error

	m.httpRequestsTotal, err = meter.Int64Counter("sub2api.http.requests",
		metric.WithDescription("Total HTTP requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	m.httpRequestDuration, err = meter.Float64Histogram("sub2api.http.request.duration",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60),
	)
	if err != nil {
		return nil, err
	}

	m.httpRequestTTFT, err = meter.Float64Histogram("sub2api.http.request.ttft",
		metric.WithDescription("Time to first token in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10),
	)
	if err != nil {
		return nil, err
	}

	m.tokensTotal, err = meter.Int64Counter("sub2api.tokens",
		metric.WithDescription("Total tokens processed"),
		metric.WithUnit("{token}"),
	)
	if err != nil {
		return nil, err
	}

	m.upstreamErrorsTotal, err = meter.Int64Counter("sub2api.upstream.errors",
		metric.WithDescription("Total upstream errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, err
	}

	m.accountFailoversTotal, err = meter.Int64Counter("sub2api.account.failovers",
		metric.WithDescription("Total account failover events"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return nil, err
	}

	m.rateLimitRejectionsTotal, err = meter.Int64Counter("sub2api.ratelimit.rejections",
		metric.WithDescription("Total rate limit rejections"),
		metric.WithUnit("{rejection}"),
	)
	if err != nil {
		return nil, err
	}

	m.concurrencyQueueDepth, err = meter.Int64Gauge("sub2api.concurrency.queue_depth",
		metric.WithDescription("Current concurrency queue depth"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	m.upstreamAccountsActive, err = meter.Int64Gauge("sub2api.upstream.accounts_active",
		metric.WithDescription("Number of active upstream accounts"),
		metric.WithUnit("{account}"),
	)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (m *Metrics) RecordRequest(ctx context.Context, method, route string, status int, platform string) {
	m.httpRequestsTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("http.method", method),
			attribute.String("http.route", route),
			attribute.Int("http.status_code", status),
			attribute.String("platform", platform),
		),
	)
}

func (m *Metrics) RecordDuration(ctx context.Context, durationSec float64, method, route string, status int, platform string) {
	m.httpRequestDuration.Record(ctx, durationSec,
		metric.WithAttributes(
			attribute.String("http.method", method),
			attribute.String("http.route", route),
			attribute.Int("http.status_code", status),
			attribute.String("platform", platform),
		),
	)
}

func (m *Metrics) RecordTTFT(ctx context.Context, ttftSec float64, platform, model string) {
	m.httpRequestTTFT.Record(ctx, ttftSec,
		metric.WithAttributes(
			attribute.String("platform", platform),
			attribute.String("model", model),
		),
	)
}

func (m *Metrics) RecordTokens(ctx context.Context, count int64, direction, platform, model string) {
	m.tokensTotal.Add(ctx, count,
		metric.WithAttributes(
			attribute.String("direction", direction),
			attribute.String("platform", platform),
			attribute.String("model", model),
		),
	)
}

func (m *Metrics) RecordUpstreamError(ctx context.Context, platform, errorKind string) {
	m.upstreamErrorsTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("platform", platform),
			attribute.String("error_kind", errorKind),
		),
	)
}

func (m *Metrics) RecordAccountFailover(ctx context.Context, platform string) {
	m.accountFailoversTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("platform", platform),
		),
	)
}

func (m *Metrics) RecordRateLimitRejection(ctx context.Context, limiterType string) {
	m.rateLimitRejectionsTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("limiter_type", limiterType),
		),
	)
}

func (m *Metrics) SetConcurrencyQueueDepth(ctx context.Context, depth int64) {
	m.concurrencyQueueDepth.Record(ctx, depth)
}

func (m *Metrics) SetUpstreamAccountsActive(ctx context.Context, count int64, platform string) {
	m.upstreamAccountsActive.Record(ctx, count,
		metric.WithAttributes(
			attribute.String("platform", platform),
		),
	)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test ./internal/pkg/otel/ -run TestMetrics -v -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/pkg/otel/metrics.go backend/internal/pkg/otel/metrics_test.go
git commit -m "feat(otel): define business metric instruments (RED, tokens, upstream errors)"
```

---

## Task 5: Wire DI Integration

**Files:**
- Create: `backend/internal/pkg/otel/wire.go`
- Modify: `backend/cmd/server/wire.go:30-57` (wire.Build), `backend/cmd/server/wire.go:70-98` (provideCleanup)

- [ ] **Step 1: Create Wire provider set**

Create `backend/internal/pkg/otel/wire.go`:

```go
package otel

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	ProvideOtel,
	ProvideMetrics,
	ProvideMetricsServer,
)
```

- [ ] **Step 2: Add provider functions to otel.go**

Add at the end of `backend/internal/pkg/otel/otel.go`:

```go
// ProvideOtel is a Wire provider that initializes the OTel SDK.
func ProvideOtel(cfg *config.Config) (*Provider, error) {
	return Init(context.Background(), &cfg.Otel)
}

// ProvideMetrics is a Wire provider for application metrics.
func ProvideMetrics() (*Metrics, error) {
	return NewMetrics()
}

// ProvideMetricsServer is a Wire provider for the internal metrics server.
func ProvideMetricsServer(cfg *config.Config, provider *Provider) *MetricsServer {
	if !cfg.Otel.Enabled {
		return nil
	}
	srv := NewMetricsServer(cfg.Otel.MetricsPort, provider.PrometheusExporter())
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
	}()
	return srv
}
```

Also add the missing imports to `otel.go` if not already present: `"net/http"` and `"log"`.

- [ ] **Step 3: Add OTel provider set to wire.Build**

In `backend/cmd/server/wire.go`, add the import:

```go
appelotel "github.com/Wei-Shaw/sub2api/internal/pkg/otel"
```

Add to `wire.Build()` after the config provider set (around line 33):

```go
// OpenTelemetry providers
appelotel.ProviderSet,
```

- [ ] **Step 4: Add OTel cleanup to provideCleanup**

In `backend/cmd/server/wire.go`, add `otelProvider *appelotel.Provider` and `metricsServer *appelotel.MetricsServer` to the `provideCleanup` function parameters.

In the cleanup function body, add OTel shutdown before the infrastructure cleanup (before Redis/Ent close):

```go
// Shutdown OTel providers
if otelProvider != nil {
	if err := otelProvider.Shutdown(ctx); err != nil {
		log.Printf("OTel provider shutdown error: %v", err)
	}
}
// Shutdown metrics server
if metricsServer != nil {
	if err := metricsServer.Shutdown(ctx); err != nil {
		log.Printf("Metrics server shutdown error: %v", err)
	}
}
```

- [ ] **Step 5: Regenerate Wire code**

Run: `cd backend && go generate ./cmd/server/`
Expected: `wire_gen.go` is regenerated without errors.

If `go generate` fails because Wire is not installed, run:
```bash
go install github.com/google/wire/cmd/wire@latest
```

- [ ] **Step 6: Verify compilation**

Run: `cd backend && go build ./cmd/server/`
Expected: Build succeeds.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/pkg/otel/wire.go backend/internal/pkg/otel/otel.go backend/cmd/server/wire.go backend/cmd/server/wire_gen.go
git commit -m "feat(otel): integrate OTel providers into Wire DI and cleanup lifecycle"
```

---

## Task 6: Gin OTel Middleware + X-Trace-Id Header

**Files:**
- Create: `backend/internal/server/middleware/otel.go`
- Modify: `backend/internal/server/router.go:54-62`

- [ ] **Step 1: Create OTel middleware file**

Create `backend/internal/server/middleware/otel.go`:

```go
package middleware

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

// TraceIDHeader adds the X-Trace-Id response header from the OTel span context.
func TraceIDHeader() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		sc := trace.SpanFromContext(c.Request.Context()).SpanContext()
		if sc.HasTraceID() {
			c.Header("X-Trace-Id", sc.TraceID().String())
		}
	}
}
```

- [ ] **Step 2: Add otelgin middleware to router**

In `backend/internal/server/router.go`, add the import:

```go
"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
```

Add after line 55 (`r.Use(middleware2.Logger())`):

```go
r.Use(otelgin.Middleware("sub2api"))
r.Use(middleware2.TraceIDHeader())
```

- [ ] **Step 3: Verify compilation**

Run: `cd backend && go build ./...`
Expected: Build succeeds.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/server/middleware/otel.go backend/internal/server/router.go
git commit -m "feat(otel): add Gin OTel auto-instrumentation and X-Trace-Id response header"
```

---

## Task 7: Logger Trace ID Injection

**Files:**
- Create: `backend/internal/pkg/logger/tracectx.go`

- [ ] **Step 1: Create trace context logger helper**

Create `backend/internal/pkg/logger/tracectx.go`:

```go
package logger

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// WithTraceContext returns a logger with trace_id and span_id fields
// extracted from the OTel span context in ctx. If no active span
// exists, returns the logger unchanged.
func WithTraceContext(ctx context.Context, l *zap.Logger) *zap.Logger {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.HasTraceID() {
		return l
	}
	return l.With(
		zap.String("trace_id", sc.TraceID().String()),
		zap.String("span_id", sc.SpanID().String()),
	)
}

// FromContextWithTrace returns the context logger enriched with trace context.
func FromContextWithTrace(ctx context.Context) *zap.Logger {
	return WithTraceContext(ctx, FromContext(ctx))
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd backend && go build ./internal/pkg/logger/...`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/pkg/logger/tracectx.go
git commit -m "feat(otel): add trace_id/span_id injection helper for Zap logger"
```

---

## Task 8: Gateway Tracing Spans

**Files:**
- Modify: `backend/internal/handler/gateway_handler.go:112` (Messages method)
- Modify: `backend/internal/service/gateway_service.go` (Forward, upstream request, response handling)

This task instruments the gateway request pipeline with OTel spans. The `otelgin` middleware already creates the root span. We add child spans for each pipeline phase.

- [ ] **Step 1: Add tracing to GatewayHandler.Messages**

In `backend/internal/handler/gateway_handler.go`, add the import:

```go
"go.opentelemetry.io/otel"
"go.opentelemetry.io/otel/attribute"
```

At the top of the `Messages` method (after line 112, before the apiKey extraction), add:

```go
ctx := c.Request.Context()
tracer := otel.Tracer("sub2api.gateway")
ctx, span := tracer.Start(ctx, "gateway.messages")
defer span.End()
c.Request = c.Request.WithContext(ctx)
```

After the API key and subject are extracted (after line 124), add span attributes:

```go
span.SetAttributes(
	attribute.Int64("user_id", subject.UserID),
	attribute.Int64("api_key_id", apiKey.ID),
)
```

After the model is parsed (after line 157), add:

```go
span.SetAttributes(
	attribute.String("model", reqModel),
	attribute.Bool("stream", reqStream),
)
```

- [ ] **Step 2: Add tracing to GatewayService.Forward**

In `backend/internal/service/gateway_service.go`, add the import:

```go
"go.opentelemetry.io/otel"
"go.opentelemetry.io/otel/attribute"
"go.opentelemetry.io/otel/codes"
```

At the beginning of the `Forward` method (line 4133), add:

```go
tracer := otel.Tracer("sub2api.gateway")
ctx, span := tracer.Start(ctx, "gateway.forward")
defer span.End()
span.SetAttributes(
	attribute.Int64("account_id", int64(account.ID)),
	attribute.String("platform", string(account.Platform)),
)
```

Update `c.Request` context if needed so child calls use the traced context.

- [ ] **Step 3: Add tracing to upstream request execution**

Find the section in `Forward` or the method that makes the actual HTTP call to the upstream provider. Wrap it with:

```go
ctx, upstreamSpan := tracer.Start(ctx, "gateway.upstream_request")
upstreamSpan.SetAttributes(
	attribute.String("upstream.url", req.URL.String()),
)
// ... execute HTTP request ...
upstreamSpan.SetAttributes(
	attribute.Int("upstream.status", resp.StatusCode),
)
upstreamSpan.End()
```

If there's a retry/failover loop, each retry should get its own span:

```go
ctx, retrySpan := tracer.Start(ctx, "gateway.upstream_request")
retrySpan.SetAttributes(
	attribute.Int("retry_attempt", attempt),
	attribute.String("failover_reason", reason),
)
// ... retry logic ...
retrySpan.End()
```

- [ ] **Step 4: Add error status to spans on failure**

In error handling paths (e.g., `handleErrorResponse`), set span status:

```go
span.SetStatus(codes.Error, "upstream error")
span.SetAttributes(attribute.Int("upstream.status", resp.StatusCode))
```

- [ ] **Step 5: Verify compilation**

Run: `cd backend && go build ./...`
Expected: Build succeeds.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handler/gateway_handler.go backend/internal/service/gateway_service.go
git commit -m "feat(otel): add tracing spans to gateway request pipeline"
```

---

## Task 9: App Helm Chart Changes

**Files:**
- Modify: `deploy/helm/sub2api/values.yaml`
- Modify: `deploy/helm/sub2api/templates/deployment.yaml`
- Modify: `deploy/helm/sub2api/templates/service.yaml`
- Modify: `deploy/helm/sub2api/templates/configmap.yaml`
- Create: `deploy/helm/sub2api/templates/servicemonitor.yaml`

- [ ] **Step 1: Add observability section to values.yaml**

Add at the end of `deploy/helm/sub2api/values.yaml`:

```yaml
## Observability (OpenTelemetry + Prometheus metrics)
observability:
  enabled: false
  otel:
    serviceName: "sub2api"
    endpoint: "http://alloy.monitoring.svc.cluster.local:4318"
    traceSampleRate: "0.1"
    metricsPort: 9090
  serviceMonitor:
    enabled: false
    interval: 15s
    labels: {}
```

- [ ] **Step 2: Add metrics container port to deployment.yaml**

In `deploy/helm/sub2api/templates/deployment.yaml`, after the `http` port definition (line 39), add:

```yaml
{{- if .Values.observability.enabled }}
  - name: metrics
    containerPort: {{ .Values.observability.otel.metricsPort }}
    protocol: TCP
{{- end }}
```

- [ ] **Step 3: Add metrics service port to service.yaml**

In `deploy/helm/sub2api/templates/service.yaml`, after the existing port block (line 13), add:

```yaml
{{- if .Values.observability.enabled }}
    - port: {{ .Values.observability.otel.metricsPort }}
      targetPort: metrics
      protocol: TCP
      name: metrics
{{- end }}
```

- [ ] **Step 4: Add OTel env vars to configmap.yaml**

In `deploy/helm/sub2api/templates/configmap.yaml`, before the closing of the data block, add:

```yaml
{{- if .Values.observability.enabled }}
  OTEL_ENABLED: "true"
  OTEL_SERVICE_NAME: {{ .Values.observability.otel.serviceName | quote }}
  OTEL_ENDPOINT: {{ .Values.observability.otel.endpoint | quote }}
  OTEL_TRACE_SAMPLE_RATE: {{ .Values.observability.otel.traceSampleRate | quote }}
  OTEL_METRICS_PORT: {{ .Values.observability.otel.metricsPort | quote }}
{{- end }}
```

- [ ] **Step 5: Create ServiceMonitor template**

Create `deploy/helm/sub2api/templates/servicemonitor.yaml`:

```yaml
{{- if and .Values.observability.enabled .Values.observability.serviceMonitor.enabled }}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: {{ include "sub2api.fullname" . }}
  labels:
    {{- include "sub2api.labels" . | nindent 4 }}
    {{- with .Values.observability.serviceMonitor.labels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
spec:
  selector:
    matchLabels:
      {{- include "sub2api.selectorLabels" . | nindent 6 }}
  endpoints:
    - port: metrics
      interval: {{ .Values.observability.serviceMonitor.interval }}
      path: /metrics
{{- end }}
```

- [ ] **Step 6: Validate Helm template rendering**

Run: `helm template test deploy/helm/sub2api/ --set observability.enabled=true --set observability.serviceMonitor.enabled=true`
Expected: Renders deployment with metrics port, service with metrics port, ServiceMonitor resource, and configmap with OTel env vars.

Also validate with observability disabled:
Run: `helm template test deploy/helm/sub2api/`
Expected: No metrics port, no ServiceMonitor, no OTel env vars.

- [ ] **Step 7: Commit**

```bash
git add deploy/helm/sub2api/values.yaml deploy/helm/sub2api/templates/deployment.yaml deploy/helm/sub2api/templates/service.yaml deploy/helm/sub2api/templates/configmap.yaml deploy/helm/sub2api/templates/servicemonitor.yaml
git commit -m "feat(helm): add observability config, metrics port, and ServiceMonitor to app chart"
```

---

## Task 10: Monitoring Helm Chart

**Files:**
- Create: `deploy/helm/monitoring/Chart.yaml`
- Create: `deploy/helm/monitoring/values.yaml`
- Create: `deploy/helm/monitoring/templates/namespace.yaml`
- Create: `deploy/helm/monitoring/templates/alloy-config.yaml`
- Create: `deploy/helm/monitoring/templates/grafana-datasources.yaml`

- [ ] **Step 1: Create Chart.yaml**

Create `deploy/helm/monitoring/Chart.yaml`:

```yaml
apiVersion: v2
name: sub2api-monitoring
description: LGTM observability stack for Sub2API (Loki, Grafana, Tempo, Prometheus)
type: application
version: 0.1.0
appVersion: "1.0.0"

dependencies:
  - name: kube-prometheus-stack
    version: "72.*"
    repository: https://prometheus-community.github.io/helm-charts
    condition: kube-prometheus-stack.enabled

  - name: tempo
    version: "1.*"
    repository: https://grafana.github.io/helm-charts
    condition: tempo.enabled

  - name: loki
    version: "6.*"
    repository: https://grafana.github.io/helm-charts
    condition: loki.enabled

  - name: alloy
    version: "0.*"
    repository: https://grafana.github.io/helm-charts
    condition: alloy.enabled
```

- [ ] **Step 2: Create values.yaml**

Create `deploy/helm/monitoring/values.yaml`:

```yaml
## Namespace for monitoring stack
namespace: monitoring

## kube-prometheus-stack (Prometheus + Grafana + Alertmanager)
kube-prometheus-stack:
  enabled: true
  prometheus:
    prometheusSpec:
      retention: 15d
      storageSpec:
        volumeClaimTemplate:
          spec:
            accessModes: ["ReadWriteOnce"]
            resources:
              requests:
                storage: 50Gi
      serviceMonitorSelectorNilUsesHelmValues: false
  grafana:
    enabled: true
    adminPassword: ""  # Set via --set or secret
    sidecar:
      dashboards:
        enabled: true
        label: grafana_dashboard
      datasources:
        enabled: true
        label: grafana_datasource
    ingress:
      enabled: false
      # Uncomment and configure for external access:
      # ingressClassName: nginx
      # hosts:
      #   - grafana.sub2api.example.com
      # tls:
      #   - secretName: grafana-tls
      #     hosts:
      #       - grafana.sub2api.example.com
  alertmanager:
    enabled: true

## Grafana Tempo (distributed tracing)
tempo:
  enabled: true
  tempo:
    storage:
      trace:
        backend: s3
        s3:
          bucket: ""      # DO Spaces bucket name
          endpoint: ""    # e.g. nyc3.digitaloceanspaces.com
          access_key: ""  # Set via --set or secret
          secret_key: ""  # Set via --set or secret
    retention: 720h  # 30 days

## Grafana Loki (log aggregation)
loki:
  enabled: true
  deploymentMode: SingleBinary
  loki:
    commonConfig:
      replication_factor: 1
    storage:
      type: s3
      s3:
        s3: ""           # s3://access_key:secret_key@endpoint/bucket
        region: ""       # e.g. nyc3
    limits_config:
      retention_period: 720h  # 30 days
  singleBinary:
    replicas: 1
    persistence:
      size: 10Gi

## Grafana Alloy (unified collection agent)
alloy:
  enabled: true
  alloy:
    configMap:
      create: false
      name: alloy-config
      key: config.alloy
```

- [ ] **Step 3: Create namespace template**

Create `deploy/helm/monitoring/templates/namespace.yaml`:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: {{ .Values.namespace }}
  labels:
    app.kubernetes.io/managed-by: {{ .Release.Service }}
```

- [ ] **Step 4: Create Alloy config**

Create `deploy/helm/monitoring/templates/alloy-config.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: alloy-config
  namespace: {{ .Values.namespace }}
data:
  config.alloy: |
    // ============================================
    // OTLP Receiver — accepts traces from the app
    // ============================================
    otelcol.receiver.otlp "default" {
      http {
        endpoint = "0.0.0.0:4318"
      }
      output {
        traces  = [otelcol.exporter.otlphttp.tempo.input]
        metrics = [otelcol.exporter.prometheus.default.input]
      }
    }

    // ============================================
    // Trace exporter → Tempo
    // ============================================
    otelcol.exporter.otlphttp "tempo" {
      client {
        endpoint = "http://{{ .Release.Name }}-tempo.{{ .Values.namespace }}.svc:4318"
      }
    }

    // ============================================
    // Metrics exporter → Prometheus remote write
    // ============================================
    otelcol.exporter.prometheus "default" {
      forward_to = [prometheus.remote_write.default.receiver]
    }

    prometheus.remote_write "default" {
      endpoint {
        url = "http://{{ .Release.Name }}-kube-prometheus-stack-prometheus.{{ .Values.namespace }}.svc:9090/api/v1/write"
      }
    }

    // ============================================
    // Log collection → Loki
    // ============================================
    discovery.kubernetes "pods" {
      role = "pod"
    }

    discovery.relabel "pod_logs" {
      targets = discovery.kubernetes.pods.targets

      rule {
        source_labels = ["__meta_kubernetes_namespace"]
        target_label  = "namespace"
      }
      rule {
        source_labels = ["__meta_kubernetes_pod_name"]
        target_label  = "pod"
      }
      rule {
        source_labels = ["__meta_kubernetes_pod_container_name"]
        target_label  = "container"
      }
      rule {
        source_labels = ["__meta_kubernetes_pod_label_app_kubernetes_io_name"]
        target_label  = "app"
      }
    }

    loki.source.kubernetes "pods" {
      targets    = discovery.relabel.pod_logs.output
      forward_to = [loki.process.json_parse.receiver]
    }

    loki.process "json_parse" {
      stage.json {
        expressions = {
          level      = "level",
          component  = "component",
          trace_id   = "trace_id",
          request_id = "request_id",
          user_id    = "user_id",
        }
      }
      stage.labels {
        values = {
          level = "",
        }
      }
      stage.structured_metadata {
        values = {
          trace_id   = "",
          request_id = "",
          component  = "",
          user_id    = "",
        }
      }
      forward_to = [loki.write.default.receiver]
    }

    loki.write "default" {
      endpoint {
        url = "http://{{ .Release.Name }}-loki.{{ .Values.namespace }}.svc:3100/loki/api/v1/push"
      }
    }
```

- [ ] **Step 5: Create Grafana data sources config**

Create `deploy/helm/monitoring/templates/grafana-datasources.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-datasources-extra
  namespace: {{ .Values.namespace }}
  labels:
    grafana_datasource: "true"
data:
  datasources.yaml: |
    apiVersion: 1
    datasources:
      - name: Tempo
        type: tempo
        uid: tempo
        url: http://{{ .Release.Name }}-tempo.{{ .Values.namespace }}.svc:3100
        access: proxy
        isDefault: false
        jsonData:
          tracesToLogsV2:
            datasourceUid: loki
            filterByTraceID: true
            filterBySpanID: false
          nodeGraph:
            enabled: true
      - name: Loki
        type: loki
        uid: loki
        url: http://{{ .Release.Name }}-loki.{{ .Values.namespace }}.svc:3100
        access: proxy
        isDefault: false
        jsonData:
          derivedFields:
            - datasourceUid: tempo
              matcherRegex: '"trace_id":"(\w+)"'
              name: TraceID
              url: '$${__value.raw}'
              urlDisplayLabel: View Trace
```

- [ ] **Step 6: Build Helm dependencies**

```bash
cd deploy/helm/monitoring && helm dependency build
```

Expected: Downloads all subchart tarballs into `charts/` directory.

- [ ] **Step 7: Validate template rendering**

Run: `helm template test deploy/helm/monitoring/`
Expected: Renders all resources without errors.

- [ ] **Step 8: Commit**

```bash
git add deploy/helm/monitoring/
git commit -m "feat(helm): add monitoring umbrella chart with kube-prometheus-stack, Tempo, Loki, Alloy"
```

---

## Task 11: Grafana Dashboards

**Files:**
- Create: `deploy/helm/monitoring/templates/grafana-dashboards.yaml`

- [ ] **Step 1: Create dashboard ConfigMap**

Create `deploy/helm/monitoring/templates/grafana-dashboards.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: sub2api-grafana-dashboards
  namespace: {{ .Values.namespace }}
  labels:
    grafana_dashboard: "true"
data:
  sub2api-overview.json: |
    {
      "annotations": { "list": [] },
      "title": "Sub2API Overview",
      "uid": "sub2api-overview",
      "version": 1,
      "timezone": "browser",
      "refresh": "30s",
      "time": { "from": "now-1h", "to": "now" },
      "templating": {
        "list": [
          {
            "name": "platform",
            "type": "query",
            "datasource": { "uid": "prometheus", "type": "prometheus" },
            "query": "label_values(sub2api_http_requests_total, platform)",
            "includeAll": true,
            "multi": true,
            "current": { "text": "All", "value": "$__all" }
          }
        ]
      },
      "panels": [
        {
          "title": "Request Rate",
          "type": "timeseries",
          "gridPos": { "h": 8, "w": 8, "x": 0, "y": 0 },
          "datasource": { "uid": "prometheus", "type": "prometheus" },
          "targets": [
            {
              "expr": "sum(rate(sub2api_http_requests_total{platform=~\"$platform\"}[5m]))",
              "legendFormat": "Total RPS"
            }
          ]
        },
        {
          "title": "Error Rate",
          "type": "timeseries",
          "gridPos": { "h": 8, "w": 8, "x": 8, "y": 0 },
          "datasource": { "uid": "prometheus", "type": "prometheus" },
          "targets": [
            {
              "expr": "sum(rate(sub2api_http_requests_total{http_status_code=~\"5..\",platform=~\"$platform\"}[5m])) / sum(rate(sub2api_http_requests_total{platform=~\"$platform\"}[5m]))",
              "legendFormat": "5xx Error Rate"
            }
          ]
        },
        {
          "title": "Latency Percentiles",
          "type": "timeseries",
          "gridPos": { "h": 8, "w": 8, "x": 16, "y": 0 },
          "datasource": { "uid": "prometheus", "type": "prometheus" },
          "targets": [
            {
              "expr": "histogram_quantile(0.50, sum(rate(sub2api_http_request_duration_seconds_bucket{platform=~\"$platform\"}[5m])) by (le))",
              "legendFormat": "p50"
            },
            {
              "expr": "histogram_quantile(0.95, sum(rate(sub2api_http_request_duration_seconds_bucket{platform=~\"$platform\"}[5m])) by (le))",
              "legendFormat": "p95"
            },
            {
              "expr": "histogram_quantile(0.99, sum(rate(sub2api_http_request_duration_seconds_bucket{platform=~\"$platform\"}[5m])) by (le))",
              "legendFormat": "p99"
            }
          ]
        },
        {
          "title": "TTFT (Time to First Token)",
          "type": "timeseries",
          "gridPos": { "h": 8, "w": 8, "x": 0, "y": 8 },
          "datasource": { "uid": "prometheus", "type": "prometheus" },
          "targets": [
            {
              "expr": "histogram_quantile(0.50, sum(rate(sub2api_http_request_ttft_seconds_bucket{platform=~\"$platform\"}[5m])) by (le))",
              "legendFormat": "p50 TTFT"
            },
            {
              "expr": "histogram_quantile(0.95, sum(rate(sub2api_http_request_ttft_seconds_bucket{platform=~\"$platform\"}[5m])) by (le))",
              "legendFormat": "p95 TTFT"
            }
          ]
        },
        {
          "title": "Token Throughput",
          "type": "timeseries",
          "gridPos": { "h": 8, "w": 8, "x": 8, "y": 8 },
          "datasource": { "uid": "prometheus", "type": "prometheus" },
          "targets": [
            {
              "expr": "sum(rate(sub2api_tokens_total{direction=\"input\",platform=~\"$platform\"}[5m]))",
              "legendFormat": "Input TPS"
            },
            {
              "expr": "sum(rate(sub2api_tokens_total{direction=\"output\",platform=~\"$platform\"}[5m]))",
              "legendFormat": "Output TPS"
            }
          ]
        },
        {
          "title": "Upstream Errors",
          "type": "timeseries",
          "gridPos": { "h": 8, "w": 8, "x": 16, "y": 8 },
          "datasource": { "uid": "prometheus", "type": "prometheus" },
          "targets": [
            {
              "expr": "sum(rate(sub2api_upstream_errors_total{platform=~\"$platform\"}[5m])) by (error_kind)",
              "legendFormat": "{{ "{{" }}error_kind{{ "}}" }}"
            }
          ]
        },
        {
          "title": "Account Failovers",
          "type": "stat",
          "gridPos": { "h": 4, "w": 6, "x": 0, "y": 16 },
          "datasource": { "uid": "prometheus", "type": "prometheus" },
          "targets": [
            {
              "expr": "sum(increase(sub2api_account_failovers_total{platform=~\"$platform\"}[1h]))",
              "legendFormat": "Last Hour"
            }
          ]
        },
        {
          "title": "Rate Limit Rejections",
          "type": "stat",
          "gridPos": { "h": 4, "w": 6, "x": 6, "y": 16 },
          "datasource": { "uid": "prometheus", "type": "prometheus" },
          "targets": [
            {
              "expr": "sum(increase(sub2api_ratelimit_rejections_total[1h]))",
              "legendFormat": "Last Hour"
            }
          ]
        },
        {
          "title": "Concurrency Queue Depth",
          "type": "gauge",
          "gridPos": { "h": 4, "w": 6, "x": 12, "y": 16 },
          "datasource": { "uid": "prometheus", "type": "prometheus" },
          "targets": [
            {
              "expr": "sub2api_concurrency_queue_depth",
              "legendFormat": "Queue Depth"
            }
          ]
        },
        {
          "title": "Active Upstream Accounts",
          "type": "stat",
          "gridPos": { "h": 4, "w": 6, "x": 18, "y": 16 },
          "datasource": { "uid": "prometheus", "type": "prometheus" },
          "targets": [
            {
              "expr": "sum(sub2api_upstream_accounts_active) by (platform)",
              "legendFormat": "{{ "{{" }}platform{{ "}}" }}"
            }
          ]
        }
      ]
    }
  sub2api-resources.json: |
    {
      "title": "Sub2API Resources",
      "uid": "sub2api-resources",
      "version": 1,
      "timezone": "browser",
      "refresh": "30s",
      "time": { "from": "now-1h", "to": "now" },
      "panels": [
        {
          "title": "Goroutines",
          "type": "timeseries",
          "gridPos": { "h": 8, "w": 12, "x": 0, "y": 0 },
          "datasource": { "uid": "prometheus", "type": "prometheus" },
          "targets": [
            {
              "expr": "process_runtime_go_goroutines{job=~\".*sub2api.*\"}",
              "legendFormat": "Goroutines"
            }
          ]
        },
        {
          "title": "Memory Usage",
          "type": "timeseries",
          "gridPos": { "h": 8, "w": 12, "x": 12, "y": 0 },
          "datasource": { "uid": "prometheus", "type": "prometheus" },
          "targets": [
            {
              "expr": "process_runtime_go_mem_heap_alloc_bytes{job=~\".*sub2api.*\"} / 1024 / 1024",
              "legendFormat": "Heap Alloc (MB)"
            },
            {
              "expr": "process_runtime_go_mem_heap_sys_bytes{job=~\".*sub2api.*\"} / 1024 / 1024",
              "legendFormat": "Heap Sys (MB)"
            }
          ]
        },
        {
          "title": "GC Pause Duration",
          "type": "timeseries",
          "gridPos": { "h": 8, "w": 12, "x": 0, "y": 8 },
          "datasource": { "uid": "prometheus", "type": "prometheus" },
          "targets": [
            {
              "expr": "rate(process_runtime_go_gc_pause_ns_total{job=~\".*sub2api.*\"}[5m]) / 1e6",
              "legendFormat": "GC Pause (ms/s)"
            }
          ]
        },
        {
          "title": "Concurrency Queue Depth",
          "type": "timeseries",
          "gridPos": { "h": 8, "w": 12, "x": 12, "y": 8 },
          "datasource": { "uid": "prometheus", "type": "prometheus" },
          "targets": [
            {
              "expr": "sub2api_concurrency_queue_depth",
              "legendFormat": "Queue Depth"
            }
          ]
        }
      ]
    }
```

- [ ] **Step 2: Validate template rendering**

Run: `helm template test deploy/helm/monitoring/`
Expected: ConfigMap with dashboard JSON renders correctly.

- [ ] **Step 3: Commit**

```bash
git add deploy/helm/monitoring/templates/grafana-dashboards.yaml
git commit -m "feat(helm): add pre-provisioned Grafana dashboards for Sub2API metrics"
```

---

## Task 12: Go Runtime Metrics + Final Integration Test

**Files:**
- Modify: `backend/internal/pkg/otel/otel.go` (add runtime metrics)

- [ ] **Step 1: Add Go runtime metrics instrumentation**

In `backend/internal/pkg/otel/otel.go`, add the import:

```go
"go.opentelemetry.io/contrib/instrumentation/runtime"
```

At the end of the `Init` function, before the return statement, add:

```go
// Start Go runtime metrics collection (goroutines, memory, GC)
if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(15 * time.Second)); err != nil {
	return nil, fmt.Errorf("starting runtime metrics: %w", err)
}
```

Add `"time"` to imports if not already present.

- [ ] **Step 2: Run all OTel package tests**

Run: `cd backend && go test ./internal/pkg/otel/ -v -count=1`
Expected: All tests PASS.

- [ ] **Step 3: Run full backend build**

Run: `cd backend && go build ./cmd/server/`
Expected: Build succeeds — all Wire injections resolve, all imports valid.

- [ ] **Step 4: Run unit tests**

Run: `cd backend && go test -tags=unit ./... 2>&1 | tail -30`
Expected: All existing tests pass. No regressions.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/pkg/otel/otel.go
git commit -m "feat(otel): add Go runtime metrics collection (goroutines, memory, GC)"
```

---

## Task 13: Documentation Update

**Files:**
- Modify: `deploy/config.example.yaml` (if it exists, add OTel config section)

- [ ] **Step 1: Add OTel section to example config**

Find the example config file and add:

```yaml
# OpenTelemetry observability (optional)
otel:
  enabled: false
  service_name: "sub2api"
  endpoint: "http://alloy.monitoring.svc:4318"
  trace_sample_rate: 0.1
  metrics_port: 9090
```

- [ ] **Step 2: Commit**

```bash
git add deploy/config.example.yaml
git commit -m "docs: add OTel configuration to example config"
```
