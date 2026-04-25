# Gateway Domain Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first shippable gateway refactor foundation: gateway-owned domain types, streaming-aware execution contracts, serializable diagnostics, and import-boundary tests.

**Architecture:** This plan implements roadmap milestone 1 only. It creates `backend/internal/gateway/domain` for gateway-owned request, plan, account, usage, billing, and diagnostic types, and `backend/internal/gateway/core` for the `Core`, `ResponseSink`, and `WebSocketConn` execution contracts. It also adds import-boundary tests so future milestones cannot accidentally pull Gin, handlers, repositories, or old service domain types into the new core.

**Tech Stack:** Go, standard library `encoding/json`, `net/http`, `go test`, repo-local packages under `backend/internal/gateway`.

---

## Scope

This plan intentionally does not migrate any live route. It creates compileable foundations that future plans will use for OpenAI Responses planning, scheduling, transport, usage, and replacement.

The approved roadmap covers the whole gateway refactor, but it is too broad for one executable implementation plan. Subsequent plans should cover these later roadmap milestones:

- OpenAI Responses characterization and ingress.
- OpenAI Responses planning and scheduler.
- OpenAI Responses HTTP/SSE replacement.
- OpenAI Responses usage/billing cleanup.
- OpenAI Responses WebSocket replacement.
- OpenAI chat/messages, Anthropic, Gemini, Antigravity, and legacy deletion.

## File Structure

- Create `backend/internal/gateway/domain/types.go`: scalar enums and common route/provider constants.
- Create `backend/internal/gateway/domain/identity.go`: gateway-owned subject, API key, user, and group snapshots.
- Create `backend/internal/gateway/domain/account.go`: account snapshots, capabilities, reservation metadata, wait plans, and scheduler decisions.
- Create `backend/internal/gateway/domain/model.go`: `CanonicalRequest`, model resolution, session input, and request mutation descriptions.
- Create `backend/internal/gateway/domain/diagnostics.go`: candidate diagnostics, rejection reasons, retry plan, attempt traces, debug plan, and safe diagnostic fingerprints.
- Create `backend/internal/gateway/domain/billing.go`: billing lifecycle plan, usage event, usage token details, and billing execution report.
- Create `backend/internal/gateway/domain/routing_plan.go`: `IngressRequest`, `RoutingPlan`, `ExecutionReport`, `GatewayError`, and helper methods.
- Create `backend/internal/gateway/domain/routing_plan_test.go`: serialization and basic default-shape tests for `RoutingPlan` and `ExecutionReport`.
- Create `backend/internal/gateway/core/contracts.go`: streaming-aware `Core`, `ResponseSink`, and `WebSocketConn` interfaces.
- Create `backend/internal/gateway/core/contracts_test.go`: compile-time fake implementations that prove the contracts are usable without Gin.
- Create `backend/internal/gateway/doc.go`: root gateway package documentation so `go list ./internal/gateway/...` includes the package deterministically.
- Create `backend/internal/gateway/import_boundary_test.go`: package import-boundary tests for `domain` and non-ingress core packages.

## Task 1: Add Domain Serialization Tests First

**Files:**
- Create: `backend/internal/gateway/domain/routing_plan_test.go`

- [ ] **Step 1: Write the failing tests**

Create `backend/internal/gateway/domain/routing_plan_test.go` with this content:

```go
package domain

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestRoutingPlanJSONRoundTrip(t *testing.T) {
	groupID := int64(42)
	accountID := int64(101)
	plan := RoutingPlan{
		RequestID: "req_123",
		Endpoint: EndpointOpenAIResponses,
		Provider: PlatformOpenAI,
		Method:   http.MethodPost,
		Path:     "/v1/responses/compact",
		Model: ModelResolution{
			RequestedModel:     "gpt-5",
			ChannelMappedModel: "gpt-5.4",
			AccountMappedModel: "gpt-5.4-mini",
			UpstreamModel:      "gpt-5.4-mini",
			BillingModel:       "gpt-5.4-mini",
			BillingModelSource: BillingModelSourceUpstream,
		},
		GroupID: &groupID,
		Session: SessionDecision{
			Key:       "session-hash",
			Source:    SessionSourcePromptCacheKey,
			StickyHit: true,
		},
		Candidates: CandidateDiagnostics{
			Total:      4,
			Eligible:   1,
			Rejected:   3,
			TopK:       2,
			LoadSkew:   0.25,
			Rejections: map[RejectionReason]int{RejectionConcurrencyFull: 2, RejectionModelUnsupported: 1},
		},
		Account: AccountDecision{
			AccountID:      &accountID,
			Layer:          AccountDecisionLayerLoadBalance,
			Reservation:    &AccountReservation{AccountID: accountID, Acquired: true},
			SelectionReason: "lowest_load",
		},
		Retry: RetryPlan{
			MaxAccountSwitches:       3,
			MaxSameAccountRetries:    1,
			RetryableStatusCodes:     []int{429, 500, 502, 503, 504},
			SameAccountBackoffMillis: 500,
		},
		Billing: BillingLifecyclePlan{
			Mode:                       BillingModeStreaming,
			RequestID:                  "bill_123",
			PayloadHash:                "sha256:abc",
			ReserveBeforeForward:       true,
			FinalizeBeforeClientCommit: true,
			ReleaseOnAccountSwitch:     true,
		},
		Debug: DebugPlan{
			SafeHeaders: map[string][]string{"content-type": {"application/json"}},
			BodyFingerprint: BodyFingerprint{
				Algorithm: "sha256",
				Value:     "abc",
				Bytes:     123,
			},
		},
	}

	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("Marshal RoutingPlan: %v", err)
	}

	var got RoutingPlan
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal RoutingPlan: %v", err)
	}

	if got.RequestID != plan.RequestID {
		t.Fatalf("RequestID = %q, want %q", got.RequestID, plan.RequestID)
	}
	if got.Model.BillingModelSource != BillingModelSourceUpstream {
		t.Fatalf("BillingModelSource = %q", got.Model.BillingModelSource)
	}
	if got.GroupID == nil || *got.GroupID != groupID {
		t.Fatalf("GroupID = %v, want %d", got.GroupID, groupID)
	}
	if got.Account.AccountID == nil || *got.Account.AccountID != accountID {
		t.Fatalf("AccountID = %v, want %d", got.Account.AccountID, accountID)
	}
	if got.Candidates.Rejections[RejectionConcurrencyFull] != 2 {
		t.Fatalf("concurrency rejection count = %d", got.Candidates.Rejections[RejectionConcurrencyFull])
	}
}

func TestExecutionReportJSONRoundTrip(t *testing.T) {
	startedAt := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(125 * time.Millisecond)

	report := ExecutionReport{
		RequestID: "req_123",
		Plan: RoutingPlan{
			RequestID: "req_123",
			Endpoint: EndpointOpenAIResponses,
			Provider: PlatformOpenAI,
		},
		Attempts: []AttemptTrace{
			{
				Attempt:           1,
				AccountID:         101,
				Transport:         TransportHTTP,
				UpstreamURL:       "https://api.openai.com/v1/responses",
				StatusCode:        http.StatusOK,
				UpstreamRequestID: "up_req_123",
				Outcome:           AttemptOutcomeSuccess,
				Terminal:          true,
				StartedAt:         startedAt,
				FinishedAt:        finishedAt,
				DurationMillis:    125,
			},
		},
		Usage: &UsageEvent{
			RequestID:        "req_123",
			APIKeyID:         7,
			UserID:           9,
			AccountID:        101,
			Provider:         PlatformOpenAI,
			Endpoint:         EndpointOpenAIResponses,
			Model:            "gpt-5",
			BillingModel:     "gpt-5",
			RequestHash:      "sha256:abc",
			InputTokens:      11,
			OutputTokens:     22,
			CacheReadTokens:  3,
			Stream:           true,
			ReasoningEffort:  "high",
			ServiceTier:      "priority",
			BillingEventKind: BillingEventKindFinalize,
		},
		Billing: BillingExecutionReport{
			Mode:          BillingModeStreaming,
			Reserved:      true,
			Finalized:     true,
			Released:      false,
			BillingEventID: "evt_123",
		},
	}

	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal ExecutionReport: %v", err)
	}

	var got ExecutionReport
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal ExecutionReport: %v", err)
	}

	if got.RequestID != "req_123" {
		t.Fatalf("RequestID = %q", got.RequestID)
	}
	if len(got.Attempts) != 1 {
		t.Fatalf("attempt count = %d", len(got.Attempts))
	}
	if got.Attempts[0].Outcome != AttemptOutcomeSuccess {
		t.Fatalf("attempt outcome = %q", got.Attempts[0].Outcome)
	}
	if got.Usage == nil || got.Usage.BillingEventKind != BillingEventKindFinalize {
		t.Fatalf("usage event = %#v", got.Usage)
	}
	if !got.Billing.Finalized {
		t.Fatalf("billing finalized = false")
	}
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run:

```bash
cd backend
go test -tags=unit ./internal/gateway/domain
```

Expected: FAIL with compile errors like `undefined: RoutingPlan`, `undefined: ExecutionReport`, and `undefined: PlatformOpenAI`.

## Task 2: Implement Gateway-Owned Domain Types

**Files:**
- Create: `backend/internal/gateway/domain/types.go`
- Create: `backend/internal/gateway/domain/identity.go`
- Create: `backend/internal/gateway/domain/account.go`
- Create: `backend/internal/gateway/domain/model.go`
- Create: `backend/internal/gateway/domain/diagnostics.go`
- Create: `backend/internal/gateway/domain/billing.go`
- Create: `backend/internal/gateway/domain/routing_plan.go`
- Test: `backend/internal/gateway/domain/routing_plan_test.go`

- [ ] **Step 1: Create scalar constants**

Create `backend/internal/gateway/domain/types.go`:

```go
package domain

type Platform string

const (
	PlatformOpenAI      Platform = "openai"
	PlatformAnthropic   Platform = "anthropic"
	PlatformGemini      Platform = "gemini"
	PlatformAntigravity Platform = "antigravity"
)

type EndpointKind string

const (
	EndpointUnknown             EndpointKind = "unknown"
	EndpointOpenAIResponses     EndpointKind = "openai_responses"
	EndpointOpenAIChat          EndpointKind = "openai_chat_completions"
	EndpointOpenAIMessages      EndpointKind = "openai_messages"
	EndpointAnthropicMessages   EndpointKind = "anthropic_messages"
	EndpointAnthropicCountToken EndpointKind = "anthropic_count_tokens"
	EndpointGeminiNative        EndpointKind = "gemini_native"
	EndpointAntigravity         EndpointKind = "antigravity"
)

type AccountType string

const (
	AccountTypeUnknown AccountType = "unknown"
	AccountTypeAPIKey  AccountType = "api_key"
	AccountTypeOAuth   AccountType = "oauth"
)

type TransportKind string

const (
	TransportUnknown   TransportKind = "unknown"
	TransportHTTP      TransportKind = "http"
	TransportSSE       TransportKind = "sse"
	TransportWebSocket TransportKind = "websocket"
)
```

- [ ] **Step 2: Create identity and policy snapshots**

Create `backend/internal/gateway/domain/identity.go`:

```go
package domain

type Subject struct {
	UserID      int64  `json:"user_id"`
	APIKeyID    int64  `json:"api_key_id"`
	Concurrency int    `json:"concurrency"`
	ClientIP    string `json:"client_ip,omitempty"`
	UserAgent   string `json:"user_agent,omitempty"`
}

type APIKeySnapshot struct {
	ID         int64           `json:"id"`
	UserID     int64           `json:"user_id"`
	GroupID    *int64          `json:"group_id,omitempty"`
	Name       string          `json:"name,omitempty"`
	RateLimits RateLimitConfig `json:"rate_limits"`
}

type UserSnapshot struct {
	ID    int64  `json:"id"`
	Email string `json:"email,omitempty"`
}

type GroupSnapshot struct {
	ID                 int64       `json:"id"`
	Name               string      `json:"name,omitempty"`
	Platform           Platform   `json:"platform"`
	DefaultMappedModel string      `json:"default_mapped_model,omitempty"`
	Policy             GroupPolicy `json:"policy"`
}

type RateLimitConfig struct {
	RequestsPerMinute int     `json:"requests_per_minute,omitempty"`
	TokensPerMinute   int     `json:"tokens_per_minute,omitempty"`
	WindowCostLimit   float64 `json:"window_cost_limit,omitempty"`
}

type GroupPolicy struct {
	RequireOAuthOnly     bool `json:"require_oauth_only,omitempty"`
	RequirePrivacySet    bool `json:"require_privacy_set,omitempty"`
	AllowMessagesDispatch bool `json:"allow_messages_dispatch,omitempty"`
}
```

- [ ] **Step 3: Create account and scheduler decision types**

Create `backend/internal/gateway/domain/account.go`:

```go
package domain

import "time"

type AccountSnapshot struct {
	ID             int64               `json:"id"`
	Name           string              `json:"name,omitempty"`
	Platform       Platform            `json:"platform"`
	Type           AccountType         `json:"type"`
	BaseURL        string              `json:"base_url,omitempty"`
	ProxyID        *int64              `json:"proxy_id,omitempty"`
	Concurrency    int                 `json:"concurrency"`
	Priority       int                 `json:"priority"`
	ModelMappings  map[string]string   `json:"model_mappings,omitempty"`
	SupportedModels []string           `json:"supported_models,omitempty"`
	Capabilities   AccountCapabilities `json:"capabilities"`
}

type AccountCapabilities struct {
	SupportsHTTP      bool `json:"supports_http"`
	SupportsSSE       bool `json:"supports_sse"`
	SupportsWebSocket bool `json:"supports_websocket"`
	SupportsOAuth     bool `json:"supports_oauth"`
}

type AccountDecisionLayer string

const (
	AccountDecisionLayerNone             AccountDecisionLayer = "none"
	AccountDecisionLayerPreviousResponse AccountDecisionLayer = "previous_response_id"
	AccountDecisionLayerSessionSticky    AccountDecisionLayer = "session_hash"
	AccountDecisionLayerLoadBalance      AccountDecisionLayer = "load_balance"
	AccountDecisionLayerWaitPlan         AccountDecisionLayer = "wait_plan"
)

type AccountDecision struct {
	AccountID       *int64               `json:"account_id,omitempty"`
	AccountName     string               `json:"account_name,omitempty"`
	AccountType     AccountType          `json:"account_type,omitempty"`
	Layer           AccountDecisionLayer `json:"layer"`
	SelectionReason string               `json:"selection_reason,omitempty"`
	Reservation     *AccountReservation  `json:"reservation,omitempty"`
	WaitPlan        *AccountWaitPlan     `json:"wait_plan,omitempty"`
}

type AccountReservation struct {
	AccountID int64  `json:"account_id"`
	Acquired  bool   `json:"acquired"`
	Token     string `json:"token,omitempty"`
}

type AccountWaitPlan struct {
	AccountID      int64         `json:"account_id"`
	MaxConcurrency int           `json:"max_concurrency"`
	Timeout        time.Duration `json:"timeout"`
	MaxWaiting     int           `json:"max_waiting"`
}
```

- [ ] **Step 4: Create model and canonical request types**

Create `backend/internal/gateway/domain/model.go`:

```go
package domain

import "net/http"

type CanonicalRequest struct {
	RequestID      string          `json:"request_id"`
	Endpoint       EndpointKind    `json:"endpoint"`
	Provider       Platform        `json:"provider"`
	RequestedModel string          `json:"requested_model"`
	Stream         bool            `json:"stream"`
	Body           []byte          `json:"-"`
	Parsed         any             `json:"-"`
	Session        SessionInput    `json:"session"`
	Headers        http.Header     `json:"headers,omitempty"`
	Subpath        string          `json:"subpath,omitempty"`
}

type ModelResolution struct {
	RequestedModel     string `json:"requested_model"`
	ChannelMappedModel string `json:"channel_mapped_model,omitempty"`
	AccountMappedModel string `json:"account_mapped_model,omitempty"`
	UpstreamModel      string `json:"upstream_model,omitempty"`
	BillingModel       string `json:"billing_model,omitempty"`
	BillingModelSource string `json:"billing_model_source,omitempty"`
}

const (
	BillingModelSourceRequested     = "requested"
	BillingModelSourceChannelMapped = "channel_mapped"
	BillingModelSourceAccountMapped = "account_mapped"
	BillingModelSourceUpstream      = "upstream"
)

type SessionInput struct {
	HeaderValue        string `json:"header_value,omitempty"`
	PromptCacheKey     string `json:"prompt_cache_key,omitempty"`
	PreviousResponseID string `json:"previous_response_id,omitempty"`
}

type SessionSource string

const (
	SessionSourceNone               SessionSource = "none"
	SessionSourceHeader             SessionSource = "header"
	SessionSourcePromptCacheKey     SessionSource = "prompt_cache_key"
	SessionSourcePreviousResponseID SessionSource = "previous_response_id"
)

type SessionDecision struct {
	Key                string        `json:"key,omitempty"`
	Source             SessionSource `json:"source"`
	StickyHit          bool          `json:"sticky_hit"`
	PreviousResponseID string        `json:"previous_response_id,omitempty"`
}

type RequestMutation struct {
	Path      string `json:"path"`
	Action    string `json:"action"`
	OldValue  string `json:"old_value,omitempty"`
	NewValue  string `json:"new_value,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Redacted  bool   `json:"redacted,omitempty"`
}
```

- [ ] **Step 5: Create diagnostics and retry types**

Create `backend/internal/gateway/domain/diagnostics.go`:

```go
package domain

import "time"

type RejectionReason string

const (
	RejectionPlatformMismatch  RejectionReason = "platform_mismatch"
	RejectionModelUnsupported  RejectionReason = "model_unsupported"
	RejectionChannelRestricted RejectionReason = "channel_restricted"
	RejectionExcluded          RejectionReason = "excluded"
	RejectionUnschedulable     RejectionReason = "unschedulable"
	RejectionQuotaExhausted    RejectionReason = "quota_exhausted"
	RejectionRPMLimited        RejectionReason = "rpm_limited"
	RejectionWindowCostLimited RejectionReason = "window_cost_limited"
	RejectionConcurrencyFull   RejectionReason = "concurrency_full"
	RejectionStickyMismatch    RejectionReason = "sticky_mismatch"
	RejectionTransportMismatch RejectionReason = "transport_mismatch"
)

type CandidateDiagnostics struct {
	Total      int                     `json:"total"`
	Eligible   int                     `json:"eligible"`
	Rejected   int                     `json:"rejected"`
	TopK       int                     `json:"top_k,omitempty"`
	LoadSkew   float64                 `json:"load_skew,omitempty"`
	Rejections map[RejectionReason]int `json:"rejections,omitempty"`
	Samples    []CandidateSample       `json:"samples,omitempty"`
}

type CandidateSample struct {
	AccountID int64           `json:"account_id"`
	Reason    RejectionReason `json:"reason,omitempty"`
	Detail    string          `json:"detail,omitempty"`
}

type RetryPlan struct {
	MaxAccountSwitches       int           `json:"max_account_switches"`
	MaxSameAccountRetries    int           `json:"max_same_account_retries"`
	RetryableStatusCodes     []int         `json:"retryable_status_codes,omitempty"`
	SameAccountBackoffMillis int           `json:"same_account_backoff_millis,omitempty"`
	TotalBudget             time.Duration `json:"total_budget,omitempty"`
}

type AttemptOutcome string

const (
	AttemptOutcomeUnknown       AttemptOutcome = "unknown"
	AttemptOutcomeSuccess       AttemptOutcome = "success"
	AttemptOutcomeRetryAccount  AttemptOutcome = "retry_account"
	AttemptOutcomeRetrySame     AttemptOutcome = "retry_same_account"
	AttemptOutcomeNonRetryable  AttemptOutcome = "non_retryable"
	AttemptOutcomeClientCanceled AttemptOutcome = "client_canceled"
)

type AttemptTrace struct {
	Attempt           int               `json:"attempt"`
	AccountID         int64             `json:"account_id,omitempty"`
	Transport         TransportKind     `json:"transport"`
	UpstreamURL       string            `json:"upstream_url,omitempty"`
	StatusCode        int               `json:"status_code,omitempty"`
	UpstreamRequestID string            `json:"upstream_request_id,omitempty"`
	RetryReason       string            `json:"retry_reason,omitempty"`
	RetryAction       string            `json:"retry_action,omitempty"`
	Mutations         []RequestMutation `json:"mutations,omitempty"`
	Outcome           AttemptOutcome    `json:"outcome"`
	Terminal          bool              `json:"terminal"`
	StartedAt         time.Time         `json:"started_at,omitempty"`
	FinishedAt        time.Time         `json:"finished_at,omitempty"`
	DurationMillis    int64             `json:"duration_millis,omitempty"`
}

type DebugPlan struct {
	SafeHeaders     map[string][]string `json:"safe_headers,omitempty"`
	BodyFingerprint BodyFingerprint    `json:"body_fingerprint,omitempty"`
	Notes           []string           `json:"notes,omitempty"`
}

type BodyFingerprint struct {
	Algorithm string `json:"algorithm,omitempty"`
	Value     string `json:"value,omitempty"`
	Bytes     int    `json:"bytes,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}
```

- [ ] **Step 6: Create billing and usage types**

Create `backend/internal/gateway/domain/billing.go`:

```go
package domain

type BillingMode string

const (
	BillingModeNone       BillingMode = "none"
	BillingModeToken      BillingMode = "token"
	BillingModeStreaming  BillingMode = "streaming"
	BillingModePerRequest BillingMode = "per_request"
	BillingModeImage      BillingMode = "image"
)

type BillingLifecyclePlan struct {
	Mode                       BillingMode `json:"mode"`
	RequestID                  string      `json:"request_id,omitempty"`
	PayloadHash                string      `json:"payload_hash,omitempty"`
	ReserveBeforeForward       bool        `json:"reserve_before_forward,omitempty"`
	FinalizeBeforeClientCommit bool        `json:"finalize_before_client_commit,omitempty"`
	ReleaseOnAccountSwitch     bool        `json:"release_on_account_switch,omitempty"`
}

type BillingEventKind string

const (
	BillingEventKindCharge   BillingEventKind = "charge"
	BillingEventKindReserve  BillingEventKind = "reserve"
	BillingEventKindFinalize BillingEventKind = "finalize"
	BillingEventKindRelease  BillingEventKind = "release"
)

type UsageEvent struct {
	RequestID        string           `json:"request_id"`
	APIKeyID         int64            `json:"api_key_id"`
	UserID           int64            `json:"user_id"`
	AccountID        int64            `json:"account_id"`
	Provider         Platform         `json:"provider"`
	Endpoint         EndpointKind     `json:"endpoint"`
	Model            string           `json:"model"`
	BillingModel     string           `json:"billing_model"`
	RequestHash      string           `json:"request_hash,omitempty"`
	InputTokens      int              `json:"input_tokens,omitempty"`
	OutputTokens     int              `json:"output_tokens,omitempty"`
	CacheReadTokens  int              `json:"cache_read_tokens,omitempty"`
	Stream           bool             `json:"stream"`
	ReasoningEffort  string           `json:"reasoning_effort,omitempty"`
	ServiceTier      string           `json:"service_tier,omitempty"`
	BillingEventKind BillingEventKind `json:"billing_event_kind"`
}

type BillingExecutionReport struct {
	Mode          BillingMode `json:"mode"`
	Reserved      bool        `json:"reserved,omitempty"`
	Finalized     bool        `json:"finalized,omitempty"`
	Released      bool        `json:"released,omitempty"`
	BillingEventID string      `json:"billing_event_id,omitempty"`
	Error          string      `json:"error,omitempty"`
}
```

- [ ] **Step 7: Create ingress, routing, and execution report types**

Create `backend/internal/gateway/domain/routing_plan.go`:

```go
package domain

import "net/http"

type IngressRequest struct {
	RequestID string         `json:"request_id"`
	Method    string         `json:"method"`
	Path      string         `json:"path"`
	Headers   http.Header    `json:"headers,omitempty"`
	Body      []byte         `json:"-"`
	ClientIP  string         `json:"client_ip,omitempty"`
	Subject   Subject        `json:"subject"`
	APIKey    APIKeySnapshot `json:"api_key"`
	User      UserSnapshot   `json:"user"`
	Group     *GroupSnapshot `json:"group,omitempty"`
	Endpoint  EndpointKind   `json:"endpoint"`
}

type RoutingPlan struct {
	RequestID  string               `json:"request_id"`
	Endpoint   EndpointKind         `json:"endpoint"`
	Provider   Platform             `json:"provider"`
	Method     string               `json:"method,omitempty"`
	Path       string               `json:"path,omitempty"`
	Model      ModelResolution      `json:"model"`
	GroupID    *int64               `json:"group_id,omitempty"`
	Session    SessionDecision      `json:"session"`
	Candidates CandidateDiagnostics `json:"candidates"`
	Account    AccountDecision      `json:"account"`
	Retry      RetryPlan            `json:"retry"`
	Billing    BillingLifecyclePlan `json:"billing"`
	Debug      DebugPlan            `json:"debug"`
}

type ExecutionReport struct {
	RequestID string                 `json:"request_id"`
	Plan      RoutingPlan            `json:"plan"`
	Attempts  []AttemptTrace         `json:"attempts,omitempty"`
	Usage     *UsageEvent            `json:"usage,omitempty"`
	Billing   BillingExecutionReport `json:"billing"`
	Error     *GatewayError          `json:"error,omitempty"`
}

type GatewayError struct {
	StatusCode int    `json:"status_code"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable,omitempty"`
}

func (p RoutingPlan) IsZero() bool {
	return p.RequestID == "" && p.Endpoint == "" && p.Provider == ""
}

func (r ExecutionReport) Succeeded() bool {
	return r.Error == nil
}
```

- [ ] **Step 8: Run domain tests**

Run:

```bash
cd backend
go test -tags=unit ./internal/gateway/domain
```

Expected: PASS.

- [ ] **Step 9: Commit domain foundation**

Run:

```bash
git add backend/internal/gateway/domain
git commit -m "feat(gateway): add domain foundation"
```

Expected: commit succeeds with only the new `backend/internal/gateway/domain` files.

## Task 3: Add Core Execution Contracts

**Files:**
- Create: `backend/internal/gateway/core/contracts.go`
- Create: `backend/internal/gateway/core/contracts_test.go`

- [ ] **Step 1: Write failing contract tests**

Create `backend/internal/gateway/core/contracts_test.go`:

```go
package core

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
)

type fakeCore struct{}

func (fakeCore) ExecuteHTTP(ctx context.Context, req domain.IngressRequest, sink ResponseSink) (*domain.ExecutionReport, error) {
	if err := sink.WriteHeader(http.StatusAccepted, http.Header{"x-test": {"ok"}}); err != nil {
		return nil, err
	}
	if err := sink.WriteChunk([]byte(`{"ok":true}`)); err != nil {
		return nil, err
	}
	if err := sink.Flush(); err != nil {
		return nil, err
	}
	return &domain.ExecutionReport{RequestID: req.RequestID}, nil
}

func (fakeCore) ExecuteWebSocket(ctx context.Context, req domain.IngressRequest, conn WebSocketConn) (*domain.ExecutionReport, error) {
	msgType, data, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	if msgType != WebSocketMessageText {
		return nil, errors.New("unexpected message type")
	}
	if err := conn.Write(ctx, WebSocketMessageText, data); err != nil {
		return nil, err
	}
	return &domain.ExecutionReport{RequestID: req.RequestID}, nil
}

type fakeSink struct {
	status    int
	header    http.Header
	body      []byte
	flushed   bool
	committed bool
}

func (s *fakeSink) WriteHeader(status int, header http.Header) error {
	s.status = status
	s.header = header.Clone()
	s.committed = true
	return nil
}

func (s *fakeSink) WriteChunk(chunk []byte) error {
	s.body = append(s.body, chunk...)
	s.committed = true
	return nil
}

func (s *fakeSink) Flush() error {
	s.flushed = true
	return nil
}

func (s *fakeSink) Committed() bool {
	return s.committed
}

type fakeWebSocketConn struct {
	readType WebSocketMessageType
	readData []byte
	wrote    []byte
	closed   bool
}

func (c *fakeWebSocketConn) Read(ctx context.Context) (WebSocketMessageType, []byte, error) {
	return c.readType, append([]byte(nil), c.readData...), nil
}

func (c *fakeWebSocketConn) Write(ctx context.Context, messageType WebSocketMessageType, data []byte) error {
	if messageType != WebSocketMessageText {
		return errors.New("unexpected write type")
	}
	c.wrote = append(c.wrote, data...)
	return nil
}

func (c *fakeWebSocketConn) Close(status WebSocketCloseStatus, reason string) error {
	c.closed = true
	return nil
}

func TestCoreContractsAreUsableWithoutGin(t *testing.T) {
	var gateway Core = fakeCore{}
	sink := &fakeSink{}

	report, err := gateway.ExecuteHTTP(context.Background(), domain.IngressRequest{RequestID: "req_1"}, sink)
	if err != nil {
		t.Fatalf("ExecuteHTTP: %v", err)
	}
	if report.RequestID != "req_1" {
		t.Fatalf("report request id = %q", report.RequestID)
	}
	if sink.status != http.StatusAccepted {
		t.Fatalf("status = %d", sink.status)
	}
	if string(sink.body) != `{"ok":true}` {
		t.Fatalf("body = %q", string(sink.body))
	}
	if !sink.flushed || !sink.Committed() {
		t.Fatalf("sink state flushed=%v committed=%v", sink.flushed, sink.Committed())
	}
}

func TestWebSocketContractIsUsableWithoutConcreteLibrary(t *testing.T) {
	var gateway Core = fakeCore{}
	conn := &fakeWebSocketConn{readType: WebSocketMessageText, readData: []byte("hello")}

	report, err := gateway.ExecuteWebSocket(context.Background(), domain.IngressRequest{RequestID: "req_ws"}, conn)
	if err != nil {
		t.Fatalf("ExecuteWebSocket: %v", err)
	}
	if report.RequestID != "req_ws" {
		t.Fatalf("report request id = %q", report.RequestID)
	}
	if string(conn.wrote) != "hello" {
		t.Fatalf("wrote = %q", string(conn.wrote))
	}
}
```

- [ ] **Step 2: Run the test and verify it fails**

Run:

```bash
cd backend
go test -tags=unit ./internal/gateway/core
```

Expected: FAIL with compile errors like `undefined: ResponseSink`, `undefined: WebSocketConn`, and `undefined: Core`.

- [ ] **Step 3: Implement core contracts**

Create `backend/internal/gateway/core/contracts.go`:

```go
package core

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
)

type Core interface {
	ExecuteHTTP(ctx context.Context, req domain.IngressRequest, sink ResponseSink) (*domain.ExecutionReport, error)
	ExecuteWebSocket(ctx context.Context, req domain.IngressRequest, conn WebSocketConn) (*domain.ExecutionReport, error)
}

type ResponseSink interface {
	WriteHeader(status int, header http.Header) error
	WriteChunk(chunk []byte) error
	Flush() error
	Committed() bool
}

type WebSocketMessageType string

const (
	WebSocketMessageText   WebSocketMessageType = "text"
	WebSocketMessageBinary WebSocketMessageType = "binary"
	WebSocketMessageClose  WebSocketMessageType = "close"
)

type WebSocketCloseStatus int

const (
	WebSocketCloseNormal          WebSocketCloseStatus = 1000
	WebSocketCloseGoingAway       WebSocketCloseStatus = 1001
	WebSocketCloseProtocolError   WebSocketCloseStatus = 1002
	WebSocketCloseUnsupportedData WebSocketCloseStatus = 1003
	WebSocketCloseInternalError   WebSocketCloseStatus = 1011
)

type WebSocketConn interface {
	Read(ctx context.Context) (WebSocketMessageType, []byte, error)
	Write(ctx context.Context, messageType WebSocketMessageType, data []byte) error
	Close(status WebSocketCloseStatus, reason string) error
}
```

- [ ] **Step 4: Run core tests**

Run:

```bash
cd backend
go test -tags=unit ./internal/gateway/core
```

Expected: PASS.

- [ ] **Step 5: Commit core contracts**

Run:

```bash
git add backend/internal/gateway/core
git commit -m "feat(gateway): add core execution contracts"
```

Expected: commit succeeds with only the new `backend/internal/gateway/core` files.

## Task 4: Add Import-Boundary Tests

**Files:**
- Create: `backend/internal/gateway/doc.go`
- Create: `backend/internal/gateway/import_boundary_test.go`

- [ ] **Step 1: Create the root gateway package doc**

Create `backend/internal/gateway/doc.go`:

```go
// Package gateway contains the new gateway-owned core packages.
package gateway
```

- [ ] **Step 2: Write boundary tests**

Create `backend/internal/gateway/import_boundary_test.go`:

```go
package gateway

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var packageImportRules = map[string][]string{
	"domain": {
		"github.com/Wei-Shaw/sub2api/internal/service",
		"github.com/Wei-Shaw/sub2api/internal/repository",
		"github.com/Wei-Shaw/sub2api/internal/handler",
		"github.com/Wei-Shaw/sub2api/internal/server",
		"github.com/gin-gonic/gin",
	},
	"core": {
		"github.com/Wei-Shaw/sub2api/internal/service",
		"github.com/Wei-Shaw/sub2api/internal/repository",
		"github.com/Wei-Shaw/sub2api/internal/handler",
		"github.com/Wei-Shaw/sub2api/internal/server",
		"github.com/gin-gonic/gin",
	},
}

func TestGatewayPackageImportBoundaries(t *testing.T) {
	root := "."
	for pkg, forbidden := range packageImportRules {
		pkgDir := filepath.Join(root, pkg)
		entries, err := os.ReadDir(pkgDir)
		if err != nil {
			t.Fatalf("read %s: %v", pkgDir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			path := filepath.Join(pkgDir, entry.Name())
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			imports := parseImports(t, path)
			for _, got := range imports {
				for _, bad := range forbidden {
					if got == bad || strings.HasPrefix(got, bad+"/") {
						t.Fatalf("%s imports forbidden package %q in %s", pkg, got, path)
					}
				}
			}
		}
	}
}

func parseImports(t *testing.T, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse imports for %s: %v", path, err)
	}
	imports := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		imports = append(imports, strings.Trim(spec.Path.Value, `"`))
	}
	return imports
}
```

- [ ] **Step 3: Run boundary tests**

Run:

```bash
cd backend
go test -tags=unit ./internal/gateway/...
```

Expected: PASS.

- [ ] **Step 4: Commit boundary tests**

Run:

```bash
git add backend/internal/gateway/doc.go backend/internal/gateway/import_boundary_test.go
git commit -m "test(gateway): enforce foundation import boundaries"
```

Expected: commit succeeds with only the root package doc and boundary test file.

## Task 5: Format, Verify, And Review

**Files:**
- Modify only files under `backend/internal/gateway`.

- [ ] **Step 1: Format the new Go files**

Run:

```bash
cd backend
gofmt -w internal/gateway
```

Expected: command exits successfully with no output.

- [ ] **Step 2: Run focused gateway tests**

Run:

```bash
cd backend
go test -tags=unit ./internal/gateway/...
```

Expected: PASS.

- [ ] **Step 3: Run package-list sanity check**

Run:

```bash
cd backend
go list ./internal/gateway/...
```

Expected output includes exactly these packages:

```text
github.com/Wei-Shaw/sub2api/internal/gateway
github.com/Wei-Shaw/sub2api/internal/gateway/core
github.com/Wei-Shaw/sub2api/internal/gateway/domain
```

- [ ] **Step 4: Inspect git diff**

Run:

```bash
git diff --stat HEAD
git diff -- backend/internal/gateway
```

Expected: only the gateway foundation files appear. There should be no changes to existing handlers, services, repositories, routes, Wire files, generated files, frontend files, or deployment files.

- [ ] **Step 5: Commit final formatting changes if needed**

If `gofmt` changed files after earlier commits, run:

```bash
git add backend/internal/gateway
git commit -m "chore(gateway): format foundation packages"
```

Expected: commit succeeds only if formatting produced changes. If `git status --short` is clean, skip this commit.

## Task 6: Prepare The Follow-Up Plan Boundary

**Files:**
- No code files.

- [ ] **Step 1: Confirm this milestone stops before route migration**

Run:

```bash
git log --oneline -5
git status --short
```

Expected: recent commits include the gateway domain/core/boundary work, and the worktree is clean.

- [ ] **Step 2: Record next-plan recommendation in implementation notes**

In the final implementation response, include this exact next-plan recommendation:

```text
Next implementation plan should be: OpenAI Responses characterization and ingress. It should add route/behavior fixtures for current Responses behavior, introduce the Gin-to-gateway ingress adapter, and still avoid replacing live forwarding paths until those fixtures exist.
```

Expected: no file changes are needed for this step.

## Self-Review Checklist

- Spec coverage:
  - Gateway-owned types from the start: covered in Tasks 1 and 2.
  - Streaming-aware execution contract: covered in Task 3.
  - Serializable `RoutingPlan` and `ExecutionReport`: covered in Tasks 1 and 2.
  - Import-boundary tests: covered in Task 4.
  - No route migration in foundation: stated in Scope and Task 6.
  - Replacement-first policy: preserved by not adding feature flags, fallback wiring, or route hooks in this foundation plan.

- Completion-marker scan:
  - No incomplete-marker text or incomplete code steps are present.
  - Follow-up work is explicitly out of this first milestone and named in Task 6.

- Type consistency:
  - `RoutingPlan`, `ExecutionReport`, `IngressRequest`, `UsageEvent`, `BillingLifecyclePlan`, `AttemptTrace`, `ResponseSink`, `WebSocketConn`, and `Core` names match across tests and implementation snippets.
  - Constants used by tests are defined in the domain snippets.
  - `ResponseSink` and `WebSocketConn` live in `core`, while gateway-owned serializable data lives in `domain`.
