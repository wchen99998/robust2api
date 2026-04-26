# Gateway Domain Foundation Implementation Plan

> **For agentic workers:** Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement this plan task-by-task.

## Goal

Build the first shippable gateway refactor foundation: gateway-owned domain types, streaming-aware execution contracts, serializable diagnostics, redaction guardrails, and import-boundary tests.

This plan implements roadmap milestone 1 only. It intentionally does not migrate any live route. It creates compilable foundations that future plans will use for OpenAI Responses planning, scheduling, transport, usage, and replacement.

## Scope

Create these packages:

- `backend/internal/gateway/domain`: gateway-owned request, plan, account, usage, billing, retry, and diagnostic types.
- `backend/internal/gateway/core`: streaming-aware executor contracts that do not import Gin or a concrete WebSocket package.
- `backend/internal/gateway`: root package documentation and import-boundary tests.

Do not change live handlers, route registration, existing gateway services, repositories, Ent schemas, or frontend code in this milestone.

## Current Foundation API Shape

The first `RoutingPlan` shape is:

```go
type RoutingPlan struct {
	Request     IngressRequest       `json:"request"`
	Subject     Subject              `json:"subject"`
	Canonical   CanonicalRequest     `json:"canonical"`
	GroupID     int64                `json:"group_id"`
	Session     SessionDecision      `json:"session"`
	Diagnostics CandidateDiagnostics `json:"diagnostics"`
	Account     AccountDecision      `json:"account"`
	Retry       RetryPlan            `json:"retry"`
	Billing     BillingLifecyclePlan `json:"billing"`
	Debug       DebugPlan            `json:"debug"`
	CreatedAt   time.Time            `json:"created_at"`
}
```

The execution report must carry the plan and allow no-usage terminal paths:

```go
type ExecutionReport struct {
	RequestID  string                 `json:"request_id"`
	Plan       RoutingPlan            `json:"plan"`
	Attempts   []AttemptTrace         `json:"attempts,omitempty"`
	Usage      *UsageEvent            `json:"usage,omitempty"`
	Billing    BillingExecutionReport `json:"billing"`
	Error      *GatewayError          `json:"error,omitempty"`
	StartedAt  time.Time              `json:"started_at"`
	FinishedAt time.Time              `json:"finished_at"`
}
```

The first core contract is:

```go
type HTTPExecutor interface {
	ExecuteHTTP(ctx context.Context, req domain.IngressRequest, sink ResponseSink) (*domain.ExecutionReport, error)
}

type WebSocketExecutor interface {
	ExecuteWebSocket(ctx context.Context, req domain.IngressRequest, conn WebSocketConn) (*domain.ExecutionReport, error)
}

type Core interface {
	HTTPExecutor
	WebSocketExecutor
}
```

## Task 1: Add Domain Serialization And Redaction Tests First

Files:

- `backend/internal/gateway/domain/routing_plan_test.go`

Steps:

- [ ] Add a `RoutingPlan` JSON round-trip test using the current nested fields: `Request`, `Subject`, `Canonical`, `Diagnostics`, `Account`, `Retry`, `Billing`, `Debug`, and `CreatedAt`.
- [ ] Add an `ExecutionReport` JSON round-trip test where the terminal attempt succeeds and `Error` is nil.
- [ ] Add a no-usage execution report test proving `plan` is serialized and nil `usage` is omitted.
- [ ] Add `ExecutionReport.Succeeded()` tests for success, explicit error, non-success terminal attempt, and no attempts.
- [ ] Add redaction tests proving request/canonical/mutation headers are redacted for auth, cookie, API key, token, secret, credential, and password-bearing names.
- [ ] Add tests proving `CanonicalRequest.Body` and `AccountReservation.Token` are not serialized into diagnostics JSON.
- [ ] Add an `IsZero` test proving semantically empty maps/slices do not make an otherwise empty `RoutingPlan` non-zero.

Run:

```bash
cd backend
go test -count=1 -tags=unit ./internal/gateway/domain
```

## Task 2: Implement Gateway-Owned Domain Types

Files:

- `backend/internal/gateway/domain/types.go`
- `backend/internal/gateway/domain/identity.go`
- `backend/internal/gateway/domain/account.go`
- `backend/internal/gateway/domain/model.go`
- `backend/internal/gateway/domain/diagnostics.go`
- `backend/internal/gateway/domain/billing.go`
- `backend/internal/gateway/domain/routing_plan.go`

Steps:

- [ ] Add scalar enum types for `Platform`, `EndpointKind`, `AccountType`, and `TransportKind`.
- [ ] Add gateway-owned identity snapshots: `Subject`, `APIKeySnapshot`, `UserSnapshot`, `GroupSnapshot`, `RateLimitConfig`, and `GroupPolicy`.
- [ ] Add account scheduling types: `AccountSnapshot`, `AccountCapabilities`, `AccountDecision`, `AccountReservation`, and `AccountWaitPlan`.
- [ ] Add model and request planning types: `CanonicalRequest`, `ModelResolution`, `SessionInput`, `SessionDecision`, and `RequestMutation`.
- [ ] Add retry and diagnostics types: `CandidateDiagnostics`, `CandidateSample`, `RetryPlan`, `AttemptTrace`, `DebugPlan`, and `BodyFingerprint`.
- [ ] Add billing and usage types: `BillingLifecyclePlan`, `UsageEvent`, `UsageTokenDetails`, and `BillingExecutionReport`.
- [ ] Add `IngressRequest`, `RoutingPlan`, `ExecutionReport`, `GatewayError`, and helper methods.
- [ ] Keep raw body bytes and reservation tokens available in memory only; do not serialize them in diagnostic JSON.
- [ ] Redact all secret-bearing headers through custom JSON marshaling for ingress, canonical, and mutation header fields.

Acceptance checks:

- `go test -count=1 -tags=unit ./internal/gateway/domain` passes.
- `go test -count=1 -tags=unit ./internal/gateway/...` still includes only the new gateway packages.

## Task 3: Add Core Execution Contracts

Files:

- `backend/internal/gateway/core/contracts.go`
- `backend/internal/gateway/core/contracts_test.go`

Steps:

- [ ] Define `HTTPExecutor`, `WebSocketExecutor`, and aggregate `Core`.
- [ ] Define `ResponseSink` with status/header/chunk/flush/committed behavior.
- [ ] Define `WebSocketConn`, `WebSocketMessageType`, and `WebSocketCloseStatus` without importing a concrete WebSocket library.
- [ ] Add fake implementations in tests that prove the contracts are usable without Gin.
- [ ] Ensure `WriteChunk` commits the fake sink, because body writes are committed response bytes even when headers were not explicitly written first.

Run:

```bash
cd backend
go test -count=1 -tags=unit ./internal/gateway/core
```

## Task 4: Add Import-Boundary Tests

Files:

- `backend/internal/gateway/doc.go`
- `backend/internal/gateway/import_boundary_test.go`

Steps:

- [ ] Add root package documentation so `go list ./internal/gateway/...` includes the package deterministically.
- [ ] Recursively scan non-test Go files under `backend/internal/gateway/domain` and `backend/internal/gateway/core`.
- [ ] Fail if either package imports Gin, handlers, server packages, existing services, repositories, Ent, config, shared internal pkg helpers, or legacy `internal/domain`.
- [ ] Ensure the forbidden-prefix logic does not accidentally reject `github.com/Wei-Shaw/sub2api/internal/gateway/domain`.

Run:

```bash
cd backend
go test -count=1 -tags=unit ./internal/gateway
```

## Task 5: Format, Verify, And Review

Run:

```bash
cd backend
gofmt -w internal/gateway
go test -count=1 -tags=unit ./internal/gateway/...
go list ./internal/gateway/...
```

Expected packages:

```text
github.com/Wei-Shaw/sub2api/internal/gateway
github.com/Wei-Shaw/sub2api/internal/gateway/core
github.com/Wei-Shaw/sub2api/internal/gateway/domain
```

Before opening a PR:

- [ ] Confirm the worktree is clean.
- [ ] Request a final code review over the full milestone range.
- [ ] Address any critical or important findings before publishing.

## Follow-Up Boundary

The next plan should cover ingress and OpenAI Responses characterization only. It should not migrate live routes until preserved behavior and intentional simplifications are explicit.
