# OpenAI Responses Native Scheduler Design

## Goal

Build the next gateway refactor phase after PR 48 by moving OpenAI Responses ingress, request planning, and account scheduling into gateway-owned packages. This phase intentionally connects the new scheduler to live HTTP/SSE and WebSocket account selection, while leaving forwarding, relay, usage recording, and billing finalization on the existing paths.

The work covers the aggressive path chosen during design review: native gateway scheduler now, not a service-backed facade.

## Context

PR 48 added the gateway-owned foundation under `backend/internal/gateway`:

- `domain` types for routing, account, model, retry, billing, usage, and diagnostics.
- `core` contracts for HTTP, WebSocket, `ResponseSink`, and `WebSocketConn`.
- serializable `RoutingPlan` and `ExecutionReport`.
- import-boundary tests for the foundation packages.

The next roadmap item is ingress and OpenAI Responses characterization. The phase is expanded here to include OpenAI Responses planning and native scheduler decisions so the new gateway boundary is exercised by real traffic before transport replacement.

Knowledge-base sources read:

- `Robust2API Knowledge Base/Index.md`
- `Robust2API Knowledge Base/Roadmaps/Gateway Refactor Roadmap.md`
- `Robust2API Knowledge Base/Milestones/Gateway Domain Foundation.md`
- `Robust2API Knowledge Base/Healthchecks/2026-04-26 - Healthcheck.md`
- `Robust2API Knowledge Base/Raw/Gateway Refactor Roadmap Design.md`
- `Robust2API Knowledge Base/Raw/Gateway Domain Foundation PR 48.md`
- `Robust2API Knowledge Base/Raw/PR 48 Review Feedback Response.md`

Repository context checked:

- `backend/internal/gateway/domain`
- `backend/internal/gateway/core`
- `backend/internal/server/routes/gateway.go`
- `backend/internal/handler/openai_gateway_handler.go`
- `backend/internal/service/openai_gateway_service.go`
- `backend/internal/service/openai_account_scheduler.go`
- `docs/superpowers/specs/2026-04-25-gateway-refactor-roadmap-design.md`
- `docs/superpowers/plans/2026-04-25-gateway-domain-foundation.md`

## Scope

In scope:

- Add gateway-owned ingress for OpenAI Responses HTTP and WebSocket planning inputs.
- Add OpenAI Responses provider parsing and validation.
- Add `RoutingPlan` construction for OpenAI Responses HTTP, SSE, compact, previous-response, and WebSocket account-selection cases.
- Add a native gateway-owned OpenAI account scheduler.
- Connect live OpenAI-platform Responses HTTP/SSE account choice to the new scheduler.
- Connect live OpenAI Responses WebSocket account choice to the new scheduler.
- Preserve old HTTP/SSE forwarding, WebSocket relay, usage recording, and billing behavior.
- Add characterization tests for preserved route behavior and OpenAI-vs-non-OpenAI dispatch.
- Expand import-boundary tests so only `gateway/ingress` and `gateway/adapters` may import Gin, config, or legacy service types.

Out of scope:

- Replacing HTTP/SSE transport.
- Replacing WebSocket relay or `WebSocketConn` transport execution.
- Moving usage extraction into `gateway/usage`.
- Moving billing reserve/finalize/release into gateway core.
- Deleting old OpenAI forwarding paths.
- Migrating chat completions, messages dispatch, Anthropic, Gemini, or Antigravity routes.

## Package Architecture

Create these packages:

```text
backend/internal/gateway/
  ingress/
  provider/openai/
  planning/
  scheduler/
  adapters/
```

`ingress` is the Gin-edge adapter. It extracts method, path, subpath, request ID, headers, body, subject/API key/group snapshots, and transport kind into `domain.IngressRequest`. It may import Gin and middleware context helpers because it is explicitly an edge package.

`provider/openai` owns OpenAI Responses request semantics. It validates and normalizes request bodies and produces `domain.CanonicalRequest` plus OpenAI-specific parse metadata needed for planning and scheduling.

`planning` builds stable `domain.RoutingPlan` values. It records route identity, model resolution intent, session decisions, retry/account-switch limits, billing lifecycle intent, debug fingerprints, and scheduler request inputs. It does not prepare upstream HTTP requests.

`scheduler` owns native account selection. It works only with gateway-owned snapshots and ports. It implements previous-response sticky, session sticky, load-aware top-k, transport compatibility, structured rejection diagnostics, wait-plan output, account slot reservation, and runtime result reporting.

`adapters` bridges current services, config, repositories, concurrency helpers, Redis-backed sticky state, channel restrictions, and old `service.Account` values into gateway-owned ports and scheduler result handles. It is the only gateway package besides `ingress` allowed to import legacy service/config types.

Hard boundaries:

- `domain`, `provider/openai`, `planning`, and `scheduler` must not import Gin, handlers, repositories, Ent, config, or legacy service models.
- `ingress` may import Gin and middleware helpers, but must not perform OpenAI semantic validation.
- `adapters` may import legacy services/config and may carry a legacy account handle for the old forwarding bridge.
- Legacy account handles must not appear in `domain`, `provider/openai`, `planning`, or `scheduler`.

## HTTP/SSE Flow

For OpenAI-platform groups on:

- `POST /responses`
- `POST /responses/*subpath`
- `POST /v1/responses`
- `POST /v1/responses/*subpath`
- `POST /openai/v1/responses`
- `POST /openai/v1/responses/*subpath`

the flow is:

1. Existing middleware authenticates, applies request body limits, sets request ID, enforces group assignment, and stores API key, auth subject, and subscription in Gin context.
2. `OpenAIGatewayHandler.Responses` reads the body once.
3. `gateway/ingress` builds `domain.IngressRequest` from Gin context, auth snapshots, request metadata, and body bytes.
4. `provider/openai` parses and validates the request body.
5. If the route is compact, the provider normalizes compact request body fields and records the compact session seed. The normalized body becomes the body used by old forwarding.
6. `planning` creates a `RoutingPlan` and scheduler request.
7. `scheduler` selects and reserves an account through gateway-owned ports.
8. The handler logs and attaches the plan summary, then uses the adapter-owned legacy account handle with existing `gatewayService.Forward`.
9. Existing response buffering, failover classification, billing eligibility, streaming reserve/finalize/release, usage recording, and response writing remain unchanged.
10. On failover/account switch, the handler reports a scheduler failure, releases any scheduler reservation, adds the failed account to exclusions, and asks the native scheduler to select again.

For non-OpenAI-platform Responses groups, existing `h.Gateway.Responses(c)` behavior remains unchanged. Tests must assert this dispatch difference.

## WebSocket Flow

For OpenAI Responses WebSocket routes:

- `GET /responses`
- `GET /v1/responses`
- `GET /openai/v1/responses`

the flow is:

1. Existing upgrade validation stays at the handler edge. A non-upgrade request keeps the current `426 invalid_request_error` response.
2. WebSocket ingress builds an `IngressRequest` with `TransportWebSocket`, route identity, headers, subject/API key/group snapshots, and client transport metadata.
3. The existing first client payload parsing path feeds the OpenAI provider parser enough data to extract model, `previous_response_id`, session input, and transport requirement.
4. `planning` builds a WebSocket `RoutingPlan`.
5. `scheduler` selects and reserves the account.
6. Existing WebSocket accept, relay, reconnect handling, payload mutation, fallback error mapping, terminal usage, and billing remain on the old path.
7. If old WebSocket retry behavior requires account reselection, the handler calls the native scheduler again with failed-account exclusions.

This phase does not replace the WebSocket relay state machine. It only moves WebSocket planning and account choice to the new gateway scheduler.

## Provider Semantics

The OpenAI provider parser preserves current client-facing validation:

- empty body: `400 invalid_request_error`
- invalid JSON: `400 invalid_request_error`
- missing model: `400 invalid_request_error`
- empty model: `400 invalid_request_error`
- non-string model: `400 invalid_request_error`
- non-boolean `stream` when present: `400 invalid_request_error`
- message-id-shaped `previous_response_id`: `400 invalid_request_error`
- compact normalization failure: `400 invalid_request_error`

The parser extracts:

- requested model
- stream flag
- compact route flag and compact subpath
- compact `prompt_cache_key` seed
- previous response ID and previous-response kind
- session input used to derive sticky session hash
- request payload hash input
- reasoning effort
- service tier
- transport kind

Function-call-output validation should move into `provider/openai` only if it can be done without importing handler or legacy service packages. If not, it remains in the handler for this phase and is documented as remaining legacy validation.

## Planning

`planning` turns ingress and provider output into a serializable `RoutingPlan`. The plan should be stable enough for fixture tests.

For this phase, the plan should include:

- endpoint kind and normalized route alias
- transport kind
- requested model
- channel-mapped model when known before scheduling
- billing model intent when known before scheduling
- stream flag
- compact path decision
- previous-response decision
- session hash decision
- scheduler request fields
- retry/account-switch policy
- billing lifecycle intent, not billing execution
- redacted header diagnostics
- bounded body fingerprint
- selected account decision after scheduling
- candidate and rejection diagnostics after scheduling

`ExecutionReport` remains partial in this phase. Full transport, usage, and billing execution reporting waits for HTTP/SSE and WebSocket replacement.

## Native Scheduler

The scheduler exposes a gateway-owned interface:

```go
type Scheduler interface {
    Select(ctx context.Context, req ScheduleRequest) (*ScheduleResult, error)
    ReportResult(ctx context.Context, accountID int64, outcome ScheduleOutcome)
}
```

The exact Go names may differ, but the responsibilities are fixed:

- previous-response sticky selection first
- session sticky selection second
- load-aware top-k selection third
- transport compatibility filtering
- excluded-account filtering
- model support filtering
- channel pricing restriction checks
- upstream channel restriction checks
- stale snapshot rechecks before final selection
- account slot reservation and release
- wait-plan generation when immediate reservation is unavailable
- structured rejection diagnostics when no account is available
- runtime success/failure/first-token reporting

Scheduler ports cover:

- list schedulable OpenAI accounts for group/run mode
- fetch account by ID
- previous-response account lookup and binding
- session sticky get, set, delete, and refresh
- channel pricing restriction checks
- upstream model restriction checks
- account load batch
- account slot acquire/release
- waiting count read/increment/decrement when needed
- current time source for testability
- runtime metrics storage for error rate and first-token latency

Selection layers are recorded as:

- `previous_response_id`
- `session_hash`
- `load_balance`

The scheduler result includes selected account snapshot, reservation, wait plan, selected layer, candidate counts, top-k, load skew, and rejection counts. The adapter result may also carry the legacy `*service.Account` handle needed by old forwarding.

## Live Handler Integration

`OpenAIGatewayHandler.Responses` should replace its account selection call with the new planning/scheduler flow. The old forwarding call stays:

```text
new ingress/provider/planning/scheduler
  -> selected legacy account handle from adapter result
  -> existing gatewayService.Forward
  -> existing usage/billing/failover response handling
```

`OpenAIGatewayHandler.ResponsesWebSocket` should use the same scheduler boundary for account choice while keeping old WebSocket relay and billing.

The handler should not start depending on scheduler internals. It should call a narrow orchestration helper that returns:

- normalized body for forwarding
- routing plan summary
- selected account handle
- reservation release function or wait plan
- scheduler report callback

This reduces churn in the large handler and keeps future transport replacement viable.

## Error Handling

Client-facing behavior stays compatible unless explicitly listed in implementation notes.

HTTP/SSE:

- Provider validation maps to existing OpenAI-style errors.
- Scheduler no-account cases return the existing `503 api_error` surface for this phase.
- Scheduler diagnostics record structured rejection counts internally.
- Account wait queue full keeps existing `429 rate_limit_error`.
- Existing forwarding, failover exhaustion, buffering, billing, and usage errors keep current behavior.

WebSocket:

- Non-upgrade requests keep current `426 invalid_request_error`.
- Scheduler no-account and wait failures map to the existing pre-relay error surface.
- Once relay starts, close/error behavior stays legacy.
- Previous-response and reconnect recovery stay legacy except account reselection goes through the native scheduler.

## Characterization And Tests

Add layered tests:

- `gateway/ingress`: route aliases, subpaths, transport kind, snapshots, request ID, and redacted headers.
- `gateway/provider/openai`: body parsing, compact normalization, model validation, stream validation, previous-response validation, session extraction, and payload hash inputs.
- `gateway/planning`: fixture tests for HTTP non-stream, HTTP stream, compact, previous-response, and WebSocket planning inputs.
- `gateway/scheduler`: previous-response sticky, session sticky, load-aware top-k, transport compatibility, excluded accounts, wait plans, no-account rejection diagnostics, reservation/release, and runtime result reporting.
- `handler`: OpenAI-platform Responses routes use the new scheduler, non-OpenAI Responses routes stay on existing compatibility handlers, failover reselection excludes failed accounts, and WebSocket planning/scheduling is live while relay remains old.
- `server/routes`: route registration and dispatch for `/responses`, `/responses/*subpath`, `/v1/responses`, `/v1/responses/*subpath`, `/openai/v1/responses`, and WebSocket upgrade routes.
- import-boundary tests: only `gateway/ingress` and `gateway/adapters` may import Gin/service/config; `domain`, `provider/openai`, `planning`, and `scheduler` stay gateway-owned.

Minimum implementation verification:

```bash
cd backend
go test -count=1 -tags=unit ./internal/gateway/...
go test -count=1 -tags=unit ./internal/handler ./internal/server/routes ./internal/service
```

If the broad handler/service command is impractical during implementation, the implementation plan must name the narrower package/test targets and explain why.

## Acceptance Criteria

- OpenAI Responses HTTP/SSE account selection uses the native gateway scheduler.
- OpenAI Responses WebSocket account selection uses the native gateway scheduler.
- Existing HTTP/SSE forwarding remains on `gatewayService.Forward`.
- Existing WebSocket relay remains on the current implementation.
- Non-OpenAI Responses groups continue using the existing generic Responses compatibility handler.
- `RoutingPlan` fixture tests cover the main HTTP/SSE and WebSocket scheduling inputs.
- Scheduler tests cover previous-response sticky, session sticky, load-aware selection, transport compatibility, exclusions, wait plans, and structured rejection diagnostics.
- Handler tests prove failover reselection excludes failed accounts through the new scheduler.
- Import-boundary tests enforce the new package ownership rules.
- The implementation PR targets `origin/main` for `wchen99998/robust2api`.

## Risks And Mitigations

Risk: scheduler rewrite changes production account choice behavior.

Mitigation: characterize current selection behavior before replacing the call site, port the algorithm with fixture tests, and preserve client-facing no-account/wait/failover surfaces.

Risk: WebSocket relay complexity leaks into the new scheduler phase.

Mitigation: only scheduling moves. Upgrade, accept, relay, reconnect payload mutation, fallback mapping, terminal usage, and billing stay legacy.

Risk: gateway-owned packages import legacy service types for convenience.

Mitigation: expand import-boundary tests and keep legacy handles inside adapters only.

Risk: the large handler becomes harder to maintain during partial migration.

Mitigation: add a narrow orchestration helper for planning/scheduling output instead of spreading new package calls throughout forwarding and billing code.

Risk: structured diagnostics expose secrets or raw bodies.

Mitigation: reuse domain redaction and body-fingerprint behavior from PR 48; never serialize raw body bytes or reservation tokens.

## Follow-Up

After this phase, the next milestone should replace OpenAI Responses HTTP/SSE transport through gateway core while reusing the ingress, provider, planning, and scheduler packages introduced here. WebSocket relay replacement remains a later milestone after HTTP/SSE stabilizes.
