# Observability: LGTM Stack Integration

**Date**: 2026-04-05
**Status**: Approved
**Branch**: `observability`

## Context

Sub2API has a mature custom observability system: metrics collection (QPS/TPS/latency percentiles/error rates), structured logging to PostgreSQL, error tracking with retry workflows, health scoring, and alerting — all with custom Vue dashboards. While comprehensive, this reimplements what Prometheus + Grafana + Tempo + Loki handle natively with better performance, ecosystem support, and tooling.

## Decision

Add the full LGTM stack (Loki, Grafana, Tempo, Mimir/Prometheus) alongside the existing system. The existing custom observability code stays untouched — this is additive. Once the new stack is validated in production, a follow-up effort removes the legacy ops tables, services, and frontend views.

## Approach

Instrument the Go app with the **OpenTelemetry SDK** using OTLP exporters. Deploy **Grafana Alloy** as the unified collection agent in the DOKS cluster. Alloy scrapes Prometheus metrics, tails pod logs to Loki, and receives OTLP traces forwarding to Tempo.

---

## 1. Infrastructure — LGTM Stack in DOKS

### Helm Charts

All deployed into a `monitoring` namespace.

| Component | Chart | Purpose |
|-----------|-------|---------|
| Prometheus | `kube-prometheus-stack` | Metrics storage, scraping, alerting (includes kube-state-metrics, node-exporter, Alertmanager) |
| Grafana | Bundled with kube-prometheus-stack | Dashboards, data source management, explore UI |
| Tempo | `grafana/tempo` (monolithic mode) | Trace storage using S3-compatible backend (DO Spaces) |
| Loki | `grafana/loki` (monolithic mode) | Log aggregation, backed by DO Spaces |
| Alloy | `grafana/alloy` | Unified collection agent — scrapes metrics, tails logs, receives OTLP traces |

### Storage

- **Tempo and Loki**: DO Spaces (S3-compatible object storage) for cost-effective long-term retention.
- **Prometheus**: Local PVCs with configurable retention (default 15 days).

### Grafana Data Sources (auto-provisioned)

- Prometheus — metrics
- Tempo — traces
- Loki — logs
- Cross-linking enabled: logs to traces (via trace ID), traces to logs (via Tempo-Loki derived field)

### Resource Placement

The monitoring stack runs on the existing DOKS node pool. If resources become tight, a dedicated `monitoring` node pool can be added via the existing Terraform `doks` module.

---

## 2. Go App Instrumentation — OTel SDK

### Dependencies

- `go.opentelemetry.io/otel` — core API
- `go.opentelemetry.io/otel/sdk/metric` — metrics SDK
- `go.opentelemetry.io/otel/sdk/trace` — tracing SDK
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` — OTLP metrics exporter
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` — OTLP trace exporter
- `go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin` — Gin auto-instrumentation
- `go.opentelemetry.io/otel/exporters/prometheus` — Prometheus `/metrics` endpoint (dual export)

### Initialization

New package `internal/pkg/otel/` handles SDK bootstrap:

- Creates `TracerProvider` with OTLP exporter (sends to Alloy)
- Creates `MeterProvider` with both OTLP exporter and Prometheus exporter
- Registers global providers
- Graceful shutdown flushes pending spans/metrics
- Wired via Wire DI, initialized in `cmd/server/main.go`

### Internal Metrics Server

A dedicated HTTP server on port `:9090` serves:
- `/metrics` — Prometheus-compatible metrics endpoint
- `/debug/pprof/*` — Go profiling (optional)

This port is **cluster-internal only** — the Kubernetes Service exposes it as ClusterIP with no Ingress rule. Alloy's ServiceMonitor targets this port for scraping.

### Configuration

```yaml
otel:
  enabled: true
  service_name: "sub2api"
  endpoint: "http://alloy.monitoring.svc:4318"  # OTLP HTTP
  trace_sample_rate: 0.1
  metrics_port: 9090
```

All overridable via env vars: `SUB2API_OTEL_ENABLED`, `SUB2API_OTEL_ENDPOINT`, `SUB2API_OTEL_TRACE_SAMPLE_RATE`, `SUB2API_OTEL_METRICS_PORT`, `SUB2API_OTEL_SERVICE_NAME`.

### Prometheus Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `sub2api_http_requests_total` | Counter | method, route, status, platform | Request count |
| `sub2api_http_request_duration_seconds` | Histogram | method, route, status, platform | Request latency |
| `sub2api_http_request_ttft_seconds` | Histogram | platform, model | Time to first token |
| `sub2api_tokens_total` | Counter | direction (input/output/cache_read/cache_creation), platform, model | Token consumption |
| `sub2api_upstream_errors_total` | Counter | platform, error_kind (429/529/5xx/timeout) | Upstream failures |
| `sub2api_account_failovers_total` | Counter | platform | Account switch events |
| `sub2api_rate_limit_rejections_total` | Counter | limiter_type | Rate limiter rejections |
| `sub2api_concurrency_queue_depth` | Gauge | — | Current queue depth |
| `sub2api_upstream_accounts_active` | Gauge | platform | Active upstream accounts |

Plus default Go runtime metrics (goroutines, GC, memory) from the OTel runtime instrumentation package.

---

## 3. Tracing — Gateway Request Flow

### Trace Structure

```
[root] HTTP POST /v1/chat/completions (otelgin auto-span)
  +-- [span] gateway.authenticate (API key validation)
  +-- [span] gateway.rate_limit (rate limiter check)
  +-- [span] gateway.select_account (sticky session + load balancing)
  |     attributes: account_id, platform, model
  +-- [span] gateway.upstream_request (HTTP call to upstream provider)
  |     attributes: upstream_url, upstream_status, retry_attempt
  |     +-- [span] gateway.upstream_request (retry, if failover occurs)
  |           attributes: failover_reason, new_account_id
  +-- [span] gateway.response_transform
  +-- [span] gateway.record_usage (async enqueue)
```

### Instrumentation Points

Spans created in `GatewayService` methods. Each major pipeline phase gets a `tracer.Start(ctx, "gateway.<phase>")` call with relevant attributes. Context propagates through the call chain for correct nesting.

### Trace ID in Logs

Zap logger enhanced to extract OTel trace ID from context and add as structured field (`trace_id`). Enables Grafana log-trace correlation.

### Trace ID in Response Headers

`X-Trace-Id` header returned to API consumers for debugging support requests.

### Sampling

Configurable via `SUB2API_OTEL_TRACE_SAMPLE_RATE` (default `0.1` = 10%). Always-sample on errors. Keeps Tempo storage reasonable while capturing all failures.

---

## 4. Log Collection — Loki via Alloy

### Mechanism

No code changes for log collection itself. The app writes structured JSON logs to stdout via Zap. Alloy runs as a DaemonSet, tails container logs, ships to Loki.

### Code Change: Trace Context in Logs

The Zap logger is enhanced to extract `trace_id` and `span_id` from OTel context and inject as top-level JSON fields. This enables Grafana's "Logs for this trace" / "Trace for this log" derived field linking.

### Loki Label Strategy

- **Stream labels** (indexed, low-cardinality): `namespace`, `pod`, `container`, `app` — from Kubernetes metadata via Alloy
- **Parsed fields** (filterable, not indexed): `level`, `component`, `trace_id`, `user_id`, `request_id` — extracted by Alloy's log processing pipeline from JSON structure

### Retention

Configurable in Loki Helm values (default 30 days). Backed by DO Spaces.

---

## 5. Grafana Dashboards and Correlation

### Pre-provisioned Dashboards (shipped as ConfigMaps)

| Dashboard | Panels |
|-----------|--------|
| Sub2API Overview | Request rate, error rate, p50/p95/p99 latency, TTFT, active accounts, health status |
| Gateway Detail | Per-platform/model breakdown, upstream error rates by kind (429/529/5xx/timeout), failover rate, token throughput |
| Resource Usage | Go runtime (goroutines, heap, GC), concurrency queue depth, rate limiter rejections |
| Upstream Accounts | Per-account request distribution, error rates, latency |

### Data Source Correlation

- **Loki to Tempo**: Derived field on `trace_id` links to Tempo trace view
- **Tempo to Loki**: Tempo data source configured with Loki search by `trace_id`
- **Prometheus to Tempo**: Exemplars on histograms link to specific traces (OTel SDK attaches trace IDs as exemplars on `sub2api_http_request_duration_seconds`)

### Alerting

Grafana alerting rules replace the custom ops alerting system (after legacy removal). Key alerts:
- Error rate > threshold (by platform)
- P99 latency > threshold
- Upstream 429 rate spike
- Pod restarts / OOM
- No data from app (scrape target down)

Notifications via email or Slack webhook, configured in Grafana contact points.

### Access

Grafana exposed via Ingress on a subdomain (e.g., `grafana.sub2api.example.com`) with its own auth, or kept cluster-internal via `kubectl port-forward`.

---

## 6. Helm Chart Changes

### App Chart (`deploy/helm/sub2api/`)

- Add second container port `metrics: 9090` to Deployment
- Add second Service port targeting `metrics` (ClusterIP only, no Ingress)
- Add `ServiceMonitor` CRD resource for Alloy/Prometheus auto-discovery
- Add `values.yaml` section for OTel config, passed as env vars
- All OTel resources gated behind `observability.enabled: true`

### Monitoring Chart (`deploy/helm/monitoring/`)

New separate Helm chart (keeps app chart clean, independent deploy/upgrade cycle):
- `kube-prometheus-stack` as dependency — Prometheus + Grafana + Alertmanager
- `grafana/tempo` — monolithic mode, DO Spaces backend
- `grafana/loki` — monolithic mode, DO Spaces backend
- `grafana/alloy` — DaemonSet config for metrics scraping, log tailing, OTLP receiving
- Dashboard ConfigMaps for pre-provisioned Grafana dashboards
- DO Spaces credentials via Kubernetes Secret

---

## 7. What Stays Unchanged (For Now)

The following existing systems remain untouched in this phase:

- `ops_metrics_collector.go` — custom system metrics collection
- `ops_error_logger.go` — custom error tracking
- `ops_system_log_sink.go` — custom log sink to PostgreSQL
- `ops_dashboard.go`, `ops_health_score.go` — custom ops dashboard
- `ops_aggregation_service.go` — pre-aggregation services
- All `ops_*` database tables and migrations
- Frontend ops dashboard (`OpsDashboard.vue`)
- Frontend admin/user usage dashboards
- Custom alerting system (alert rules, events, silences)

These are removed in a follow-up effort after the LGTM stack is validated.

---

## 8. Future Work (Out of Scope)

- Remove legacy ops observability code and tables
- Add deeper tracing spans (DB queries, Redis operations, background jobs)
- Dedicated monitoring node pool in DOKS
- Grafana Cloud migration (if self-hosting becomes burdensome)
