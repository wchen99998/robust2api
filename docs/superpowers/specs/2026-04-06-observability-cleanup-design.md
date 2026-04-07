# Observability Cleanup & Enrichment

## Problem Statement

Live cluster investigation (2026-04-06) on the LGTM stack revealed five issues:

1. **Duplicate metrics** — Both Prometheus scrape (ServiceMonitor) and OTLP push (app → Alloy → remote-write) produce the same `sub2api_*` metrics with different label sets.
2. **Intermittent OTLP metric export failures** — `failed to upload metrics: context deadline exceeded` from the gRPC metric exporter to Alloy.
3. **Traces dominated by health check noise** — Tempo contains only `GET /health` traces from kube-probe; zero business traces visible.
4. **No trace_id in structured logs** — Access logs lack `trace_id`/`span_id` fields despite code existing, breaking Loki-to-Tempo correlation.
5. **No DB/Redis trace instrumentation** — Only Gin HTTP spans exist; no visibility into database or cache operations within a request.

## Decisions

| Question | Decision |
|----------|----------|
| Metrics ingestion path | Prometheus scrape only; remove OTLP metric export |
| Trace span richness | Gateway auto-instrumented spans + DB + Redis child spans |
| Health check traces | Filter at otelgin level (`WithFilter`) |
| Trace sample rate | Keep at 0.1 (10%) |
| Non-gateway route metrics | Keep as-is (only gateway routes with platform) |
| Log trace correlation fix | Reorder middleware (otelgin before RequestLogger) |

## Changes

### 1. Single Metrics Path — Remove OTLP Metric Export

**Files:**
- `backend/internal/pkg/otel/otel.go` — Remove `otlpmetricgrpc` exporter. Keep only `promexporter` as the sole reader on the MeterProvider.
- `backend/go.mod` — Remove `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc` dependency.
- `deploy/helm/monitoring/templates/alloy-config.yaml` — Remove `otelcol.exporter.prometheus` and `prometheus.remote_write` blocks. Alloy only handles traces + logs.

**Rationale:** Eliminates duplicate metrics and the intermittent gRPC timeout errors. Prometheus scrape via ServiceMonitor is the standard k8s pattern and already works reliably with richer pod-level labels.

### 2. Health Check Trace Filter

**Files:**
- `backend/internal/server/router.go` — Add `otelgin.WithFilter(func)` option that returns false for `/health` and `/setup/status` paths.

**Rationale:** Health check traffic from kubelet (~6 req/min) dominates trace storage at 10% sampling. These traces have zero diagnostic value (100% `kube-probe` user agent, always 200).

### 3. Fix trace_id in Logs — Middleware Reorder

**Files:**
- `backend/internal/server/router.go` — Change middleware registration order:

Before:
```
RequestLogger()        (1)
Logger()               (2)
otelgin.Middleware()   (3)
TraceIDHeader()        (4)
RequestMetrics()       (5)
```

After:
```
otelgin.Middleware()   (1)  — creates span with trace_id
RequestLogger()        (2)  — logger picks up span context
Logger()               (3)  — access log includes trace_id/span_id
TraceIDHeader()        (4)
RequestMetrics()       (5)
```

**Rationale:** The otelgin middleware must create the span before any logging middleware reads the context. After this change, `logger.FromContext(ctx)` calls `WithTraceContext()` which finds the active span and enriches logs with `trace_id` and `span_id`. Alloy's existing JSON parser already extracts these fields for Loki structured metadata, enabling Loki-to-Tempo trace correlation.

### 4. DB Instrumentation via otelsql

**Files:**
- `backend/go.mod` — Add `github.com/XSAM/otelsql` dependency.
- `backend/internal/repository/ent.go` — Wrap the SQL driver with `otelsql.Open()` before passing to Ent:
  ```go
  db, err := otelsql.Open("postgres", dsn)
  // register db stats for metrics
  otelsql.RegisterDBStatsMetrics(db)
  drv := entsql.OpenDB(dialect.Postgres, db)
  ```

**Spans produced:** `db.query`, `db.exec`, `db.prepare` with `db.statement`, `db.system=postgresql` attributes.

### 5. Redis Instrumentation via redisotel

**Files:**
- `backend/go.mod` — Add `github.com/redis/go-redis/extra/redisotel/v9` dependency.
- `backend/internal/repository/redis.go` — Add tracing hook after client creation:
  ```go
  client := redis.NewClient(buildRedisOptions(cfg))
  redisotel.InstrumentTracing(client)
  ```

**Spans produced:** One span per Redis command (e.g. `redis - get`, `redis - set`) with `db.system=redis`, `db.statement` attributes.

### 6. Alloy Config — Remove Metrics Pipeline

**Files:**
- `deploy/helm/monitoring/templates/alloy-config.yaml`

**Before:** OTLP receiver outputs traces to Tempo AND metrics to Prometheus.
**After:** OTLP receiver outputs traces to Tempo only. Metrics pipeline removed entirely.

Resulting signal flow:
```
App → OTLP/gRPC → Alloy → Tempo        (traces)
Pod logs → Alloy → Loki                 (logs, with trace_id extraction)
Prometheus → scrape /metrics directly    (metrics)
```

## Out of Scope

- Custom gateway-stage spans (account selection, upstream call, response streaming) — can add later if otelgin + DB/Redis spans prove insufficient.
- Grafana dashboards — separate task after instrumentation is confirmed working.
- Alerting rules — separate task.
- Changing the trace sample rate — keep at 0.1.
- Recording metrics for non-gateway routes — otelgin auto-instrumentation covers basic HTTP metrics for all routes.
