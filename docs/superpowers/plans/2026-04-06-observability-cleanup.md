# Observability Cleanup & Enrichment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix duplicate metrics, health check trace noise, missing log trace correlation, and add DB/Redis span instrumentation.

**Architecture:** Remove OTLP metric push path (keep Prometheus scrape only), reorder Gin middleware so otelgin runs first (enables trace_id in logs), filter health check routes from tracing, wrap SQL driver with otelsql and Redis client with redisotel for child spans.

**Tech Stack:** Go, OpenTelemetry SDK v1.43, otelgin v0.67, otelsql, redisotel, Grafana Alloy, Helm

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `backend/internal/pkg/otel/otel.go` | Modify | Remove OTLP metric exporter, keep only trace exporter + Prometheus exporter |
| `backend/internal/server/router.go` | Modify | Reorder middleware (otelgin first), add health check filter |
| `backend/internal/repository/ent.go` | Modify | Wrap SQL driver with otelsql |
| `backend/internal/repository/redis.go` | Modify | Add redisotel tracing hook |
| `backend/go.mod` | Modify | Add otelsql + redisotel, remove otlpmetricgrpc |
| `deploy/helm/monitoring/templates/alloy-config.yaml` | Modify | Remove metrics pipeline, keep traces + logs only |

---

### Task 1: Remove OTLP Metric Exporter from OTel Init

**Files:**
- Modify: `backend/internal/pkg/otel/otel.go`
- Verify: `backend/internal/pkg/otel/otel_test.go` (existing tests still pass, no changes needed)

- [ ] **Step 1: Update `otel.go` — remove OTLP metric exporter**

Remove the `otlpmetricgrpc` import and the metric exporter creation. The `MeterProvider` should only have the `promexporter` as its reader.

Replace the entire file `backend/internal/pkg/otel/otel.go` with:

```go
package otel

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// runtimeStartOnce ensures runtime.Start is called at most once across all Init invocations
// (e.g. multiple test cases that each call Init).
var runtimeStartOnce sync.Once

// Provider holds the initialized OTel providers and exposes a Shutdown method.
type Provider struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	promExporter   *promexporter.Exporter
	enabled        bool
}

func (p *Provider) TracerProvider() *sdktrace.TracerProvider {
	return p.tracerProvider
}

func (p *Provider) MeterProvider() *sdkmetric.MeterProvider {
	return p.meterProvider
}

func (p *Provider) PrometheusExporter() *promexporter.Exporter {
	return p.promExporter
}

func (p *Provider) Shutdown(ctx context.Context) error {
	if !p.enabled {
		return nil
	}
	if p.tracerProvider != nil {
		_ = p.tracerProvider.Shutdown(ctx)
	}
	if p.meterProvider != nil {
		_ = p.meterProvider.Shutdown(ctx)
	}
	return nil
}

// Init initializes OTel tracing and metrics providers.
// Traces are exported via OTLP/gRPC to the configured endpoint.
// Metrics are exposed via Prometheus exporter only (scraped by ServiceMonitor).
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
	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithInsecure(),
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

	// --- Meter (Prometheus scrape only, no OTLP push) ---
	promExp, err := promexporter.New()
	if err != nil {
		return nil, fmt.Errorf("creating prometheus exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(promExp),
	)
	otel.SetMeterProvider(mp)

	// Start Go runtime metrics collection (goroutines, memory, GC).
	var runtimeErr error
	runtimeStartOnce.Do(func() {
		runtimeErr = runtime.Start(runtime.WithMinimumReadMemStatsInterval(15 * time.Second))
	})
	if runtimeErr != nil {
		return nil, fmt.Errorf("starting runtime metrics: %w", runtimeErr)
	}

	return &Provider{
		tracerProvider: tp,
		meterProvider:  mp,
		promExporter:   promExp,
		enabled:        true,
	}, nil
}

// ProvideOtel is a Wire provider that initializes the OTel SDK.
func ProvideOtel(cfg *config.Config) (*Provider, error) {
	return Init(context.Background(), &cfg.Otel)
}

// ProvideMetricsServer is a Wire provider for the internal metrics server.
func ProvideMetricsServer(cfg *config.Config, provider *Provider) *MetricsServer {
	if !cfg.Otel.Enabled {
		return nil
	}
	return NewMetricsServer(cfg.Otel.MetricsPort, provider.PrometheusExporter())
}
```

- [ ] **Step 2: Run existing tests to verify nothing breaks**

Run: `cd backend && go test -tags=unit ./internal/pkg/otel/ -v`
Expected: both `TestInit_Disabled` and `TestInit_Enabled_NoEndpoint` PASS.

- [ ] **Step 3: Remove `otlpmetricgrpc` from go.mod**

```bash
cd backend && go mod tidy
```

Verify that `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc` no longer appears in `go.mod` (it may remain in `go.sum` — that's fine).

- [ ] **Step 4: Run tests again after go mod tidy**

Run: `cd backend && go test -tags=unit ./internal/pkg/otel/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd backend && git add internal/pkg/otel/otel.go go.mod go.sum
git commit -m "refactor(otel): remove OTLP metric exporter, keep Prometheus scrape only

Eliminates duplicate metrics (OTLP push + Prometheus scrape) and fixes
intermittent gRPC timeout errors on metric upload."
```

---

### Task 2: Reorder Middleware + Filter Health Check Traces

**Files:**
- Modify: `backend/internal/server/router.go`

- [ ] **Step 1: Update middleware order and add health check filter**

In `backend/internal/server/router.go`, replace the middleware registration block (lines 55–61):

Old:
```go
	// 应用中间件
	r.Use(middleware2.RequestLogger())
	r.Use(middleware2.Logger())
	if cfg.Otel.Enabled {
		r.Use(otelgin.Middleware("sub2api"))
		r.Use(middleware2.TraceIDHeader())
		r.Use(middleware2.RequestMetrics())
	}
```

New:
```go
	// 应用中间件
	// OTel middleware MUST run before logging middleware so that trace_id/span_id
	// are available in structured log output for Loki→Tempo correlation.
	if cfg.Otel.Enabled {
		r.Use(otelgin.Middleware("sub2api",
			otelgin.WithFilter(func(r *http.Request) bool {
				p := r.URL.Path
				return p != "/health" && p != "/setup/status"
			}),
		))
	}
	r.Use(middleware2.RequestLogger())
	r.Use(middleware2.Logger())
	if cfg.Otel.Enabled {
		r.Use(middleware2.TraceIDHeader())
		r.Use(middleware2.RequestMetrics())
	}
```

Also add `"net/http"` to the import block in `router.go` (it's not currently imported).

- [ ] **Step 2: Verify compilation**

Run: `cd backend && go build ./internal/server/`
Expected: compiles with no errors.

- [ ] **Step 3: Run full unit test suite**

Run: `cd backend && go test -tags=unit ./internal/server/ -v`
Expected: PASS (or no test files — either way, no compilation error).

- [ ] **Step 4: Commit**

```bash
cd backend && git add internal/server/router.go
git commit -m "fix(otel): reorder middleware and filter health check traces

Move otelgin.Middleware before RequestLogger/Logger so trace_id appears
in structured logs. Filter /health and /setup/status from tracing to
eliminate health check noise in Tempo."
```

---

### Task 3: Add PostgreSQL Instrumentation via otelsql

**Files:**
- Modify: `backend/go.mod`
- Modify: `backend/internal/repository/ent.go`

- [ ] **Step 1: Add otelsql dependency**

```bash
cd backend && go get github.com/XSAM/otelsql
```

- [ ] **Step 2: Update `ent.go` to wrap SQL driver with otelsql**

In `backend/internal/repository/ent.go`, replace the current driver opening code.

Old imports:
```go
import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/migrations"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/lib/pq" // PostgreSQL 驱动，通过副作用导入注册驱动
)
```

New imports:
```go
import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/migrations"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/lib/pq" // PostgreSQL 驱动，通过副作用导入注册驱动
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)
```

Replace lines 50-54 (the driver opening block):

Old:
```go
	drv, err := entsql.Open(dialect.Postgres, dsn)
	if err != nil {
		return nil, nil, err
	}
	applyDBPoolSettings(drv.DB(), cfg)
```

New:
```go
	// Wrap the SQL driver with OpenTelemetry instrumentation.
	// This produces child spans for every DB query (db.query, db.exec).
	db, err := otelsql.Open("postgres", dsn,
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
	)
	if err != nil {
		return nil, nil, err
	}
	applyDBPoolSettings(db, cfg)
	if err := otelsql.RegisterDBStatsMetrics(db); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("registering db stats metrics: %w", err)
	}
	drv := entsql.OpenDB(dialect.Postgres, db)
```

Note: `applyDBPoolSettings` accepts `*sql.DB` — check the signature. The current code passes `drv.DB()` which returns `*sql.DB`. With the new code, we pass `db` directly (which is `*sql.DB`). The function signature is `func applyDBPoolSettings(db *sql.DB, cfg *config.Config)` — this is compatible.

- [ ] **Step 3: Run go mod tidy**

```bash
cd backend && go mod tidy
```

- [ ] **Step 4: Verify compilation**

Run: `cd backend && go build ./internal/repository/`
Expected: compiles with no errors.

- [ ] **Step 5: Run unit tests**

Run: `cd backend && go test -tags=unit ./internal/repository/ -v`
Expected: PASS. Existing tests use `entsql.OpenDB` with sqlite/sqlmock and don't go through `InitEnt`, so they are unaffected.

- [ ] **Step 6: Commit**

```bash
cd backend && git add internal/repository/ent.go go.mod go.sum
git commit -m "feat(otel): add PostgreSQL query tracing via otelsql

Wraps the database/sql driver with otelsql instrumentation. Every DB
query now produces child spans with db.statement and db.system attributes,
visible in Tempo trace waterfall."
```

---

### Task 4: Add Redis Instrumentation via redisotel

**Files:**
- Modify: `backend/go.mod`
- Modify: `backend/internal/repository/redis.go`

- [ ] **Step 1: Add redisotel dependency**

```bash
cd backend && go get github.com/redis/go-redis/extra/redisotel/v9
```

- [ ] **Step 2: Update `redis.go` to add tracing hook**

In `backend/internal/repository/redis.go`, add the redisotel import and instrument the client.

Old imports:
```go
import (
	"crypto/tls"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"

	"github.com/redis/go-redis/v9"
)
```

New imports:
```go
import (
	"crypto/tls"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)
```

Replace the `InitRedis` function:

Old:
```go
func InitRedis(cfg *config.Config) *redis.Client {
	return redis.NewClient(buildRedisOptions(cfg))
}
```

New:
```go
func InitRedis(cfg *config.Config) *redis.Client {
	client := redis.NewClient(buildRedisOptions(cfg))
	// Add OpenTelemetry tracing hook — produces a child span per Redis command.
	if err := redisotel.InstrumentTracing(client); err != nil {
		// Non-fatal: tracing is best-effort. Log and continue.
		// The client works fine without the hook.
		_ = err
	}
	return client
}
```

- [ ] **Step 3: Run go mod tidy**

```bash
cd backend && go mod tidy
```

- [ ] **Step 4: Verify compilation**

Run: `cd backend && go build ./internal/repository/`
Expected: compiles with no errors.

- [ ] **Step 5: Run unit tests**

Run: `cd backend && go test -tags=unit ./internal/repository/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd backend && git add internal/repository/redis.go go.mod go.sum
git commit -m "feat(otel): add Redis command tracing via redisotel

Instruments go-redis client with OpenTelemetry tracing hook. Every Redis
command now produces a child span with db.system=redis and db.statement
attributes."
```

---

### Task 5: Update Alloy Config — Remove Metrics Pipeline

**Files:**
- Modify: `deploy/helm/monitoring/templates/alloy-config.yaml`

- [ ] **Step 1: Update Alloy config template**

Replace the full content of `deploy/helm/monitoring/templates/alloy-config.yaml` with:

```yaml
{{- if .Values.alloy.enabled }}
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
      grpc {
        endpoint = "0.0.0.0:4317"
      }
      output {
        traces = [otelcol.exporter.otlphttp.tempo.input]
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
{{- end }}
```

Key change: the `otelcol.receiver.otlp` `output` block no longer includes `metrics`. The entire `otelcol.exporter.prometheus` and `prometheus.remote_write` blocks are removed.

- [ ] **Step 2: Validate Helm template renders**

```bash
cd deploy/helm/monitoring && helm template monitoring . --set alloy.enabled=true --set namespace=monitoring | grep -A5 "otelcol.receiver.otlp"
```

Expected: output block shows only `traces = [...]`, no `metrics` line.

- [ ] **Step 3: Commit**

```bash
git add deploy/helm/monitoring/templates/alloy-config.yaml
git commit -m "refactor(monitoring): remove metrics pipeline from Alloy config

Alloy now only handles traces (→ Tempo) and logs (→ Loki). Metrics flow
exclusively through Prometheus scrape via ServiceMonitor, eliminating the
duplicate ingestion path."
```

---

### Task 6: Final Verification

- [ ] **Step 1: Full compilation check**

```bash
cd backend && go build ./...
```

Expected: no errors.

- [ ] **Step 2: Full unit test suite**

```bash
cd backend && go test -tags=unit ./... -count=1
```

Expected: all tests PASS.

- [ ] **Step 3: Lint check**

```bash
cd backend && golangci-lint run ./...
```

Expected: no new errors.

- [ ] **Step 4: Verify go.mod is clean**

```bash
cd backend && go mod tidy && git diff go.mod go.sum
```

Expected: no diff (already tidy).

- [ ] **Step 5: Final commit if any fixups needed**

Only if previous steps required fixes:
```bash
git add -A && git commit -m "chore: fixup lint and build issues from observability cleanup"
```
