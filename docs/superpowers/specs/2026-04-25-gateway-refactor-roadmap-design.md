# Gateway Refactor Roadmap Design

## Goal

Refactor the Sub2API gateway into a simpler, gateway-owned architecture that replaces the current handler/service-heavy request paths while preserving the broad gateway-facing contract. The roadmap covers the whole gateway migration, with OpenAI Responses as the first full end-to-end replacement target.

## Decision Summary

- Scope: whole gateway refactor roadmap.
- Priority: internal simplification first.
- Intentional behavior changes: document the breaking or simplification list with rationale and migration notes.
- Admin/internal APIs: may change freely when that simplifies the gateway design, with required frontend/admin updates in the same milestone.
- Domain boundary: gateway-owned types from the start.
- First migration target: OpenAI Responses.
- Rollout style: replacement over dual-path rollout. The old path may remain while a milestone is being developed, but migrated routes should delete or make old route code unreachable in the same PR.
- Verification bar: focused unit and characterization tests per route migration. Fake-upstream integration tests are encouraged for high-risk flows but not mandatory for every migration PR.
- GitHub target: implementation branches and PRs target `origin/main` for `wchen99998/robust2api`, not `upstream`.

## Context

The current gateway has grown around large handler and service paths. OpenAI Responses is the most important first target because it combines route normalization, request validation, compact aliases, channel/model mapping, account scheduling, HTTP/SSE forwarding, WebSocket forwarding, retry/failover behavior, usage extraction, durable billing lifecycle, observability, and compatibility behavior.

Recent work already added gateway runtime helpers, durable billing lifecycle hooks, and OpenAI WSv2 simplifications. The new roadmap should build on those lessons, but should not preserve old coupling just because it exists. The new architecture should make request decisions explicit and testable before forwarding.

## Architecture Boundaries

The new gateway core lives under `backend/internal/gateway` and owns its domain language. Core packages should not pass `service.Account`, `service.APIKey`, `service.Group`, or `gin.Context` across boundaries. Existing service/domain types can appear only in adapter packages whose job is to translate current repositories, services, and config into gateway-owned snapshots and ports.

Proposed package layout:

```text
backend/internal/gateway/
  core/           request execution orchestration and route-independent state machines
  domain/         gateway-owned types: Platform, Subject, AccountSnapshot, RoutingPlan, UsageEvent
  ingress/        Gin-to-gateway request adapter, endpoint normalization, body/header extraction
  planning/       model, group, channel, session, billing, retry, and route planning
  scheduler/      account candidate filtering, reservation, wait plans, rejection diagnostics
  provider/       provider adapters, with provider/openai first
  transport/      HTTP, SSE, and WebSocket upstream execution over gateway abstractions
  usage/          usage extraction and canonical billing event construction
  observability/  redaction, bounded fingerprints, plan diagnostics, attempt diagnostics
  adapters/       bridges from existing service/repository/config code into gateway ports
```

Hard boundaries:

- `domain` does not import `service`, `repository`, Gin, handler packages, or provider packages.
- `core`, `planning`, `scheduler`, `provider`, `transport`, `usage`, and `observability` do not import Gin or handler packages.
- `provider/*` does not import handler packages.
- `scheduler` does not perform HTTP or WebSocket forwarding.
- `usage` does not select accounts.
- Handlers become ingress glue: they authenticate through existing middleware, adapt request context into gateway input, invoke the new core, and write through a generic response sink.

`RoutingPlan` is the central artifact. It is not only a log object; it is the contract between parsing, planning, scheduler, provider preparation, retry, billing lifecycle, usage, and observability. It must be serializable and stable enough for fixture tests.

## Core Execution Contract

A plain `Handle(ctx, req) (*GatewayResult, error)` contract is not enough because gateway routes include JSON responses, SSE streams, and WebSocket relays. OpenAI Responses streams bytes while collecting terminal usage, tracking first-token latency, preserving billing lifecycle, and reacting to client disconnects.

The core should expose streaming-aware execution:

```go
type Core interface {
    ExecuteHTTP(ctx context.Context, req IngressRequest, sink ResponseSink) (*ExecutionReport, error)
    ExecuteWebSocket(ctx context.Context, req IngressRequest, conn WebSocketConn) (*ExecutionReport, error)
}
```

`ResponseSink` is a gateway-owned interface for status, headers, body chunks, flush, and “has anything been committed?” checks. The Gin handler owns the concrete implementation, but core and transport only see the interface.

`WebSocketConn` is a gateway-owned abstraction over the client WebSocket. It should expose read, write, close, and close-status behavior without importing the concrete WebSocket library into core.

## Request Flow

The high-level flow is:

```text
Gin route + middleware
  -> ingress builds IngressRequest with gateway-owned subject/API key/group snapshots
  -> provider parses body into CanonicalRequest
  -> planner creates RoutingPlan
  -> scheduler selects/reserves account or returns structured no-account diagnostics
  -> provider prepares UpstreamRequest
  -> executor runs RetryPlan over transport attempts
  -> transport writes response through ResponseSink while producing UpstreamResult or stream report
  -> usage extracts UsageEvent from final result or terminal stream frames
  -> billing lifecycle publishes reserve/finalize/release as required
  -> ExecutionReport returns RoutingPlan, attempts, diagnostics, usage, and billing outcome
```

Ownership rules:

- Retry/failover lives in `core` as one state machine using `RetryPlan`.
- Provider-specific request/body/header mutation lives in provider adapters.
- HTTP/SSE/WebSocket byte movement lives in `transport`.
- Account reservation release is owned by core and must be tied to every terminal path: success, retry, account switch, billing failure, panic recovery, and context cancellation.
- Billing lifecycle is part of execution planning. Queue-first and streaming reserve/finalize behavior affect when the client response may be committed.
- `ExecutionReport` is diagnostic and test-facing. It should include `RoutingPlan`, attempt traces, selected account summary, usage summary, billing lifecycle result, and major simplification decisions.

## Gateway-Owned Domain Types

The exact field list will evolve during implementation, but the foundation should define gateway-owned types in this direction:

```go
type Platform string
type EndpointKind string
type AccountType string

type Subject struct {
    UserID int64
    APIKeyID int64
    Concurrency int
}

type APIKeySnapshot struct {
    ID int64
    UserID int64
    GroupID *int64
    RateLimits RateLimitConfig
}

type GroupSnapshot struct {
    ID int64
    Platform Platform
    DefaultMappedModel string
    AdminPolicy GroupPolicy
}

type AccountSnapshot struct {
    ID int64
    Name string
    Platform Platform
    Type AccountType
    BaseURL string
    Concurrency int
    ModelMappings map[string]string
    Capabilities AccountCapabilities
}

type ModelResolution struct {
    RequestedModel string
    ChannelMappedModel string
    AccountMappedModel string
    UpstreamModel string
    BillingModel string
    BillingModelSource string
}

type RoutingPlan struct {
    RequestID string
    Endpoint EndpointKind
    Provider Platform
    Model ModelResolution
    GroupID *int64
    Session SessionDecision
    Candidates CandidateDiagnostics
    Account AccountDecision
    Retry RetryPlan
    Billing BillingLifecyclePlan
    Debug DebugPlan
}
```

Adapter packages may translate existing service models into these snapshots. Core packages should not accept existing service models directly.

## Compatibility And Simplification Policy

The roadmap prioritizes internal simplification while preserving the broad gateway-facing contract. The default is “same public endpoint, same general response shape, same authentication and billing semantics,” but edge behaviors can change when removing them significantly simplifies the gateway.

Policy:

- Preserve stable client-facing routes unless a milestone explicitly removes or changes them.
- Treat `/responses` and `/responses/*subpath` as first-class preserved aliases alongside `/v1/responses` and `/openai/v1/responses`.
- Keep non-OpenAI groups on existing generic Responses compatibility until their provider migration.
- Admin/internal APIs may change freely when that simplifies gateway architecture. Required frontend/admin updates belong in the same milestone.
- Intentional behavior changes do not require dual-path flags or compatibility fixtures. They require a documented breaking/simplification list with rationale and migration notes.
- Do not preserve undocumented bugs just because tests can capture them.
- Prefer deleting old migrated code over retaining feature flags, shadow execution, or permanent fallbacks.
- Keep route-level compatibility tests for behaviors explicitly preserved.
- Minimum verification per migration PR is focused unit and characterization tests.

OpenAI Responses simplification candidates:

- Reduce ad hoc debug/header/body logging in favor of redacted diagnostics and bounded body fingerprints.
- Normalize model resolution into `ModelResolution` rather than passing raw model strings between layers.
- Replace unstructured “no available accounts” failures with scheduler rejection diagnostics.
- Collapse scattered retry loops into one execution state machine.
- Remove route-specific handler billing construction once usage events are canonical.
- Simplify WebSocket fallback/reconnect edge behavior where preserving it would keep the old tangled state machine alive.

## Roadmap Milestones

### 1. Gateway Domain Foundation

Create `internal/gateway/domain`, core request/result contracts, `ResponseSink`, `WebSocketConn`, `RoutingPlan`, `AttemptTrace`, `UsageEvent`, gateway-owned snapshots, and import-boundary tests. This milestone does not migrate real routes yet.

Acceptance criteria:

- Gateway domain/core packages compile without importing Gin, handlers, repositories, or existing service domain models.
- `RoutingPlan` and `ExecutionReport` are serializable.
- Import-boundary tests enforce the intended direction.

### 2. Ingress And OpenAI Responses Characterization

Add route and behavior fixtures for current OpenAI Responses behavior. Cover `/responses`, `/responses/*subpath`, `/v1/responses`, `/v1/responses/*subpath`, `/openai/v1/responses`, `/openai/v1/responses/*subpath`, compact paths, previous-response validation, stream/non-stream handling, and WebSocket upgrade validation.

Intentional simplifications should be listed in the milestone notes instead of preserved by default.

Acceptance criteria:

- Preserved route behavior has focused tests.
- The simplification list names behaviors that will not be carried forward.
- OpenAI-platform and non-OpenAI-platform dispatch differences are covered.

### 3. OpenAI Responses Planning And Scheduling

Implement provider parsing, route normalization, model resolution, channel mapping, billing plan, retry plan, and account scheduling for OpenAI Responses using gateway-owned types.

Scheduler output must include:

- selected account decision
- reservation or wait-plan metadata
- structured rejection counts
- sticky previous-response/session decisions
- candidate count/top-k/load diagnostics where applicable

Acceptance criteria:

- `RoutingPlan` fixtures cover major OpenAI Responses routing cases.
- Scheduler tests cover deterministic choices and rejection diagnostics.
- Account reservation lifecycle is represented in gateway-owned types.

### 4. OpenAI Responses HTTP/SSE Replacement

Replace POST OpenAI Responses routes for OpenAI-platform groups with the new core for non-stream and SSE. Delete or make unreachable the old OpenAI Responses HTTP forwarding path for those routes in the same milestone. Non-OpenAI Responses compatibility routes remain on their existing generic path until their own provider migration.

Acceptance criteria:

- Migrated handlers do not perform routing, account selection, provider mutation, retry, forwarding, or billing construction.
- HTTP JSON and SSE behavior match documented preserved behavior.
- Old OpenAI Responses HTTP forwarding is unused for migrated OpenAI-platform routes.
- Focused tests pass for route dispatch, request preparation, response handling, and failure handling.

### 5. OpenAI Responses Billing And Usage Cleanup

Move Responses usage extraction and event construction fully into `gateway/usage`. Cover request payload hash, billing model source, service tier, reasoning effort, terminal stream usage, incomplete stream policy, queue-first billing, and streaming reserve/finalize/release.

This milestone may be folded into milestone 4 if implementation shows billing lifecycle cannot be safely separated from HTTP/SSE replacement.

Acceptance criteria:

- Usage extraction tests cover non-stream JSON, SSE terminal frames, incomplete streams, and WebSocket-ready terminal billing structures.
- Billing lifecycle timing is explicit in `BillingLifecyclePlan`.
- Handler-level Responses usage construction is removed for migrated routes.

### 6. OpenAI Responses WebSocket Replacement

Replace WebSocket handling for `GET /responses`, `GET /v1/responses`, and `GET /openai/v1/responses` through the new core. Preserve only documented WebSocket behavior. Simplify edge-case reconnect/fallback behavior where the milestone notes explicitly say so. Delete old migrated WebSocket handler/service paths.

Acceptance criteria:

- WebSocket ingress uses gateway-owned `WebSocketConn`.
- Attempt traces include WebSocket retries, terminal errors, and billing outcomes.
- Old migrated WebSocket handler/service paths are deleted or unreachable.
- Focused WebSocket tests cover upgrade validation, previous-response validation, success relay, terminal billing, and selected simplification cases.

### 7. OpenAI Chat Completions And Messages Dispatch

Migrate OpenAI-compatible `/v1/chat/completions` and `/v1/messages` dispatch after Responses stabilizes. Reuse the same core contracts and provider adapter patterns.

Acceptance criteria:

- Route-specific compatibility transformations are provider-owned.
- Billing/usage flows use canonical gateway usage events.
- Old migrated OpenAI chat/messages route paths are deleted or unreachable.

### 8. Anthropic Messages And Count Tokens

Migrate native Anthropic `/v1/messages` and `/v1/messages/count_tokens`, including pass-through, compatibility transforms, failover, streaming, and billing.

Acceptance criteria:

- Anthropic provider adapter owns request/response compatibility.
- Shared scheduler/core/transport behavior is reused rather than copied.
- Preserved Anthropic route behavior has focused tests.

### 9. Gemini Native And Antigravity

Migrate Gemini native and Antigravity last because they have more provider-specific compatibility behavior and should benefit from abstractions proven by earlier providers.

Acceptance criteria:

- Provider-specific compatibility remains isolated in provider adapters.
- Existing route dispatch semantics are either preserved or explicitly simplified.
- Generic core code does not gain Gemini or Antigravity-specific branches.

### 10. Gateway Legacy Deletion

Remove old gateway service paths, stale helper functions, and compatibility shims only after all owned routes have migrated. Keep behavior docs and selected fixtures as regression protection.

Acceptance criteria:

- No migrated route depends on the old gateway forwarding services.
- Old helper functions are deleted unless still used by non-migrated code.
- Import-boundary tests remain clean.

## Test Strategy

Roadmap-level test layers:

- Import-boundary tests: enforce no Gin/service/repository imports in gateway-owned domain/core packages, and no handler imports from providers.
- `RoutingPlan` fixture tests: stable JSON fixtures for route normalization, model resolution, channel mapping, billing model source, session decisions, retry plans, and scheduler candidate diagnostics.
- Provider unit tests: OpenAI first, covering request preparation, headers, OAuth vs API key behavior, compact path handling, base URL suffix construction, body mutations, and error classification.
- Scheduler unit tests: deterministic account choice and structured rejection counts for unsupported model, group/channel restriction, excluded account, unschedulable account, quota/RPM/window limits, concurrency full, sticky mismatch, and transport mismatch.
- Transport tests: fake upstream tests for non-stream JSON, SSE, upstream 4xx/5xx, network errors, response body limits, client disconnect handling, and WebSocket upgrade/relay behavior.
- Usage tests: non-stream JSON usage, SSE terminal usage, incomplete stream policy, WebSocket terminal billing, request payload hash, service tier, reasoning effort, and billing model source.
- Handler/route tests: verify migrated routes invoke the new core and old route path is deleted or unreachable.

Regression commands should scale with risk. Early foundation PRs can run focused package tests. Large deletion milestones should run broader backend unit tests and lint when practical.

## Route Migration Acceptance Criteria

Each migrated route is complete only when:

- The route’s documented preserved behavior has tests.
- Intentional behavior changes are listed in the spec or milestone notes.
- The handler does not perform routing, account selection, provider request mutation, retry, forwarding, or billing construction for that migrated route.
- The migrated route no longer uses the old forwarding path.
- Core packages keep the import boundaries.
- `ExecutionReport` exposes enough plan/attempt/billing diagnostics to debug failures without raw secret-bearing logs.

## Risks And Mitigations

### Risk: Foundation Becomes Speculative

Mitigation: foundation types should be only as broad as needed to migrate OpenAI Responses first. Future providers can extend the contracts after the first replacement proves them.

### Risk: Gateway-Owned Types Duplicate Too Much Service Logic

Mitigation: adapters can translate from current service/repository types while migration is in progress, but core packages should depend only on gateway-owned snapshots and ports.

### Risk: Billing Timing Regressions

Mitigation: billing lifecycle is part of planning and execution from the start. Queue-first and streaming reserve/finalize/release behavior must be represented in `BillingLifecyclePlan` before route replacement.

### Risk: Streaming And WebSocket Semantics Leak Back Into Handlers

Mitigation: `ResponseSink` and `WebSocketConn` exist specifically to keep byte movement out of handlers while avoiding full buffering of streams.

### Risk: Replacement Without Dual-Path Rollout Misses Edge Cases

Mitigation: keep focused route/characterization tests for documented preserved behavior, require explicit simplification notes for intentionally removed behavior, and keep the first target limited to OpenAI-platform Responses routes.

## Out Of Scope

- Rewriting admin UI purely for visual or workflow cleanup.
- Preserving every undocumented legacy edge case.
- Moving all providers at once.
- Replacing PostgreSQL, Redis, Ent, Gin, or Wire.
- Introducing permanent feature flags for old/new gateway behavior.

## Open Follow-Ups For Implementation Planning

- Decide whether OpenAI Responses usage cleanup is a separate milestone or part of HTTP/SSE replacement after inspecting implementation coupling.
- Define exact `ResponseSink` and `WebSocketConn` method sets.
- Define import-boundary test implementation.
- Define the initial OpenAI Responses simplification list before the first replacement PR.
- Decide the first concrete PR split after this roadmap spec is approved.
