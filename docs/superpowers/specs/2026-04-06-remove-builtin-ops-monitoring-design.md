# Remove Built-in Ops Monitoring System

**Date:** 2026-04-06
**Status:** Approved
**Branch:** `observability`

## Context

Sub2API has a proper external observability stack deployed (Prometheus + Grafana + Tempo + Loki + Alertmanager via `deploy/helm/monitoring/`). The built-in ops monitoring system — custom dashboards, metrics aggregation, alerting, system logs — is now redundant and adds maintenance burden. This spec covers its removal.

## Decision

Remove all built-in observability features that the external LGTM stack replaces. Keep only app-specific operational actions that Grafana cannot provide.

## What Gets Removed

### Replaced by Grafana + Prometheus
- Dashboard metrics (QPS, throughput, latency charts, error trends, error distribution)
- Dashboard snapshots (cached overview queries)
- OpenAI token stats card
- WebSocket real-time QPS/TPS streaming
- Real-time traffic summary

### Replaced by Prometheus + Alertmanager
- Alert rule CRUD (create/update/delete rules)
- Alert event tracking and status management
- Alert silence management
- Alert evaluation background service
- Email notification configuration for alerts
- Metric threshold settings
- Alert runtime settings

### Replaced by Loki
- System log ingestion, viewing, and cleanup
- System log sink background service

### Replaced by Prometheus time-series storage
- Hourly/daily metrics pre-aggregation background service
- Metrics cleanup background service
- System metrics snapshot collection
- Job heartbeat tracking
- Scheduled report generation
- Health score calculations
- Window stats queries

### `opsMonitoringEnabled` setting
- The feature gate for the old dashboard is removed. Retained endpoints (errors, concurrency, runtime config) are always available to admins.

## What Gets Kept

### 1. Error Drill-down + Retry
- **Why:** Unique business logic. Loki can show error logs, but cannot re-execute a failed upstream API call. Admins need to view error details and retry specific requests.
- **Endpoints kept:**
  - `GET /admin/ops/request-errors` — list request-level errors
  - `GET /admin/ops/request-errors/:id` — error detail
  - `POST /admin/ops/request-errors/:id/retry-client` — retry from client perspective
  - `POST /admin/ops/request-errors/:id/upstream-errors/:idx/retry` — retry specific upstream attempt
  - `PUT /admin/ops/request-errors/:id/resolve` — mark resolved
  - `GET /admin/ops/upstream-errors` — list upstream errors
  - `GET /admin/ops/upstream-errors/:id` — upstream error detail
  - `POST /admin/ops/upstream-errors/:id/retry` — retry upstream call
  - `PUT /admin/ops/upstream-errors/:id/resolve` — mark resolved
  - `GET /admin/ops/requests` — request drilldown (success + error)
- **Tables kept:** `ops_error_logs`, `ops_retry_attempts`

### 2. Concurrency & Account Availability
- **Why:** Live application state from Redis showing which specific accounts are at capacity. Useful in the admin panel where operators manage accounts, avoiding context-switch to Grafana.
- **Endpoints kept:**
  - `GET /admin/ops/concurrency` — concurrency slot stats
  - `GET /admin/ops/user-concurrency` — per-user concurrency
  - `GET /admin/ops/account-availability` — which accounts are available/at-capacity

### 3. Runtime Log Configuration
- **Why:** App configuration, not monitoring. Allows changing log level at runtime without restarting the pod.
- **Endpoints kept:**
  - `GET /admin/ops/runtime/logging` — current log config
  - `PUT /admin/ops/runtime/logging` — update log level
  - `POST /admin/ops/runtime/logging/reset` — reset to default

## Database Changes

### New migration: drop unused tables

```sql
-- Drop in dependency order (events before rules)
DROP TABLE IF EXISTS ops_system_log_cleanup_audits;
DROP TABLE IF EXISTS ops_system_logs;
DROP TABLE IF EXISTS ops_alert_silences;
DROP TABLE IF EXISTS ops_alert_events;
DROP TABLE IF EXISTS ops_alert_rules;
DROP TABLE IF EXISTS ops_job_heartbeats;
DROP TABLE IF EXISTS ops_system_metrics;
DROP TABLE IF EXISTS ops_metrics_daily;
DROP TABLE IF EXISTS ops_metrics_hourly;
```

### Tables kept
- `ops_error_logs` — error details for drill-down
- `ops_retry_attempts` — retry audit trail

### Old migrations
Left untouched. They are historical record and do not affect the running system.

## Backend Files

### Files to delete (~30 files)

**Services:**
- `service/ops_metrics_collector.go`
- `service/ops_aggregation_service.go`
- `service/ops_cleanup_service.go`
- `service/ops_scheduled_report_service.go`
- `service/ops_alert_evaluator_service.go`
- `service/ops_health_score.go`
- `service/ops_health_score_test.go`
- `service/ops_histograms.go`
- `service/ops_trends.go`
- `service/ops_window_stats.go`
- `service/ops_dashboard.go`
- `service/ops_dashboard_models.go`
- `service/ops_realtime_traffic.go`
- `service/ops_query_mode.go`
- `service/ops_query_mode_test.go`
- `service/ops_repo_mock_test.go` (mocks deleted interface methods)

**Repositories:**
- `repository/ops_repo_preagg.go`
- `repository/ops_repo_dashboard.go`
- `repository/ops_repo_trends.go`
- `repository/ops_repo_histograms.go`
- `repository/ops_repo_window_stats.go`
- `repository/ops_repo_realtime_traffic.go`
- `repository/ops_repo_openai_token_stats.go`

**Handlers:**
- `handler/admin/ops_dashboard_handler.go`
- `handler/admin/ops_snapshot_v2_handler.go`

### Files to modify

**`service/ops_port.go`** — Slim `OpsRepository` interface to only error log + retry methods. Remove all dashboard, alert, system log, metrics, heartbeat methods.

**`service/wire.go`** — Remove Wire providers for deleted services: `ProvideOpsMetricsCollector`, `ProvideOpsAggregationService`, `ProvideOpsAlertEvaluatorService`, `ProvideOpsCleanupService`, `ProvideOpsScheduledReportService`, `ProvideOpsSystemLogSink`.

**`cmd/server/wire.go`** — Remove deleted background services from the Wire injector and shutdown sequence.

**`handler/admin/ops_realtime_handler.go`** — Keep concurrency + availability methods. Remove `GetRealtimeTrafficSummary` if it queries deleted tables.

**`handler/handler.go`** — `Admin.Ops` field stays (handler is slimmed, not removed).

**`handler/wire.go`** — Keep `NewOpsHandler`, but its dependencies shrink.

**`server/routes/admin.go`** — Gut `registerOpsRoutes` to only register kept endpoints (~15 routes instead of ~40).

**`config/config.go`** — Remove any ops-specific config fields that are no longer needed (alert evaluation intervals, metrics collection intervals, etc.).

**`handler/dto/settings.go`** — Remove `opsMonitoringEnabled` from settings DTOs if present.

**`service/setting_service.go`** / **`service/settings_view.go`** — Remove ops monitoring toggle from settings logic.

## Frontend Files

### Files to delete (~20 files)

**Components:**
- `views/admin/ops/components/OpsDashboardHeader.vue`
- `views/admin/ops/components/OpsDashboardSkeleton.vue`
- `views/admin/ops/components/OpsThroughputTrendChart.vue`
- `views/admin/ops/components/OpsLatencyChart.vue`
- `views/admin/ops/components/OpsErrorTrendChart.vue`
- `views/admin/ops/components/OpsErrorDistributionChart.vue`
- `views/admin/ops/components/OpsSwitchRateTrendChart.vue`
- `views/admin/ops/components/OpsOpenAITokenStatsCard.vue`
- `views/admin/ops/components/OpsAlertRulesCard.vue`
- `views/admin/ops/components/OpsAlertEventsCard.vue`
- `views/admin/ops/components/OpsEmailNotificationCard.vue`
- `views/admin/ops/components/OpsSystemLogTable.vue`
- `views/admin/ops/components/OpsSettingsDialog.vue`
- `views/admin/ops/components/OpsRuntimeSettingsCard.vue` (alert settings only — deleted)

**Tests:**
- `views/admin/ops/components/__tests__/OpsOpenAITokenStatsCard.spec.ts`

### Files to modify

**`views/admin/ops/OpsDashboard.vue`** — Rewrite to show only: error log table, concurrency card, account availability card, runtime log config. Remove all chart/alert/system-log sections.

**`api/admin/ops.ts`** — Remove all deleted endpoint functions (~25 functions). Keep error, concurrency, availability, and runtime log config functions (~15 functions).

**`components/layout/AppSidebar.vue`** — Remove `opsMonitoringEnabled` conditional. Always show the ops nav item (or rename it to "Operations").

**`views/admin/ops/types.ts`** — Remove types for deleted features.

**Router** — Keep the `/admin/ops` route as-is.

## Implementation Order

1. **Backend services + repos**: Delete service and repository files for removed features
2. **Slim interfaces**: Update `OpsRepository` interface and its implementation
3. **Wire cleanup**: Remove providers and background service shutdown code, regenerate Wire
4. **Handlers + routes**: Delete/modify handlers, gut route registration
5. **Settings cleanup**: Remove `opsMonitoringEnabled` and related config
6. **Database migration**: Add migration to drop unused tables
7. **Frontend cleanup**: Delete components, slim dashboard, trim API client
8. **Tests**: Remove/update ops-related test files
9. **Verify**: `go build`, `golangci-lint`, `pnpm build` all pass

## Risks

- **Compilation cascade**: Removing interfaces and services will cause compilation errors in many files. Must be done carefully with Wire regeneration.
- **Settings migration**: Removing `opsMonitoringEnabled` from settings view needs frontend + backend coordination.
- **Error logging writes**: The gateway flow writes to `ops_error_logs`. This code path must be preserved even as surrounding ops code is deleted.

## Success Criteria

- All removed endpoints return 404
- Kept endpoints (errors, concurrency, availability, runtime log config) work as before
- No ops background services running (metrics collector, aggregation, cleanup, alerts, reports, system log sink)
- `go build`, `golangci-lint run ./...`, `pnpm build` all pass
- Database migration drops 9 tables cleanly
