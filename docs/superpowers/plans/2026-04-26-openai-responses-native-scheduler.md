# OpenAI Responses Native Scheduler Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move OpenAI Responses HTTP/SSE and WebSocket account selection to gateway-owned ingress, planning, and native scheduler packages while keeping existing forwarding, relay, usage, and billing paths intact.

**Architecture:** Add focused gateway packages for ingress adaptation, OpenAI Responses parsing, plan construction, and native account scheduling. Legacy services are reached only through `gateway/adapters` and a narrow exported bridge on `OpenAIGatewayService`; `domain`, `provider/openai`, `planning`, and `scheduler` stay free of Gin and legacy service types.

**Tech Stack:** Go 1.x, Gin, existing Sub2API service layer, gateway domain/core packages from PR 48, `gjson`, standard `net/http`, unit tests with `testing` and `testify/require`.

---

## File Structure

Create:

- `backend/internal/gateway/ingress/openai_responses.go` - Gin-to-gateway adapter for OpenAI Responses HTTP and WebSocket planning inputs.
- `backend/internal/gateway/ingress/openai_responses_test.go` - route alias, subpath, transport, subject snapshot, request ID, and header redaction tests.
- `backend/internal/gateway/provider/openai/responses.go` - OpenAI Responses parser, validation, compact normalization, and parse metadata.
- `backend/internal/gateway/provider/openai/responses_test.go` - parser and validation tests.
- `backend/internal/gateway/planning/openai_responses.go` - OpenAI Responses `RoutingPlan` builder.
- `backend/internal/gateway/planning/openai_responses_test.go` - stable plan fixture tests.
- `backend/internal/gateway/scheduler/openai.go` - gateway-owned scheduler types, ports, selection algorithm, diagnostics, and runtime reporting.
- `backend/internal/gateway/scheduler/openai_test.go` - scheduler behavior tests.
- `backend/internal/gateway/adapters/openai_scheduler.go` - bridge from existing `OpenAIGatewayService` and legacy accounts into scheduler ports/results.
- `backend/internal/gateway/adapters/openai_scheduler_test.go` - adapter mapping tests that do not exercise full service behavior.
- `backend/internal/handler/openai_responses_planning.go` - narrow handler orchestration helper that calls ingress, provider, planning, scheduler, and adapter result mapping.
- `backend/internal/handler/openai_responses_planning_test.go` - handler-level tests proving live scheduler integration without replacing forwarding.

Modify:

- `backend/internal/gateway/domain/routing_plan.go` - add non-serialized body bytes to `IngressRequest`, and optional normalized subpath/route alias fields if needed by plan fixtures.
- `backend/internal/gateway/domain/identity.go` - add platform/default mapped model to `GroupSnapshot` only if planning needs it.
- `backend/internal/gateway/domain/diagnostics.go` - add scheduler diagnostic fields only if existing `CandidateDiagnostics` cannot express needed rejection counts.
- `backend/internal/gateway/import_boundary_test.go` - expand package boundary checks.
- `backend/internal/handler/openai_gateway_handler.go` - replace HTTP/SSE and WebSocket account-selection call sites with the planning helper while keeping old forwarding and relay.
- `backend/internal/service/openai_account_scheduler.go` - keep existing scheduler for non-migrated paths until deleted later; add exported bridge methods only when `gateway/adapters` needs access to existing private behavior.
- `backend/internal/service/openai_gateway_service.go` - add exported bridge methods for sticky/session/channel/account access only if they cannot be implemented from existing public methods.
- `backend/internal/server/routes/gateway_test.go` - add/adjust route dispatch tests for OpenAI and non-OpenAI Responses aliases.

## Task 1: Domain Additions And Import Boundary Expansion

**Files:**
- Modify: `backend/internal/gateway/domain/routing_plan.go`
- Modify: `backend/internal/gateway/domain/identity.go`
- Modify: `backend/internal/gateway/import_boundary_test.go`
- Test: `backend/internal/gateway/domain/routing_plan_test.go`

- [ ] **Step 1: Add failing domain tests for non-serialized ingress body and group platform**

Append these tests to `backend/internal/gateway/domain/routing_plan_test.go`:

```go
func TestIngressRequestBodyIsNotSerialized(t *testing.T) {
	req := IngressRequest{
		RequestID: "req-body",
		Endpoint:  EndpointOpenAIResponses,
		Platform:  PlatformOpenAI,
		Transport: TransportHTTP,
		Method:    http.MethodPost,
		Path:      "/v1/responses",
		Body:      []byte(`{"model":"gpt-5.1","input":"secret prompt"}`),
	}

	raw, err := json.Marshal(req)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "secret prompt")
	require.NotContains(t, string(raw), "gpt-5.1")
}

func TestSubjectGroupSnapshotCarriesPlatform(t *testing.T) {
	subject := Subject{
		Group: GroupSnapshot{
			ID:       10,
			Name:     "openai-group",
			Platform: PlatformOpenAI,
		},
	}

	raw, err := json.Marshal(subject)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"platform":"openai"`)
}
```

- [ ] **Step 2: Run the failing domain tests**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/gateway/domain -run 'TestIngressRequestBodyIsNotSerialized|TestSubjectGroupSnapshotCarriesPlatform'
```

Expected: fail because `IngressRequest.Body` and `GroupSnapshot.Platform` do not exist.

- [ ] **Step 3: Add minimal domain fields**

Modify `backend/internal/gateway/domain/routing_plan.go`:

```go
type IngressRequest struct {
	RequestID string        `json:"request_id"`
	Endpoint  EndpointKind  `json:"endpoint"`
	Platform  Platform      `json:"platform"`
	Transport TransportKind `json:"transport"`
	Method    string        `json:"method"`
	Path      string        `json:"path"`
	Subpath   string        `json:"subpath,omitempty"`
	Header    http.Header   `json:"header,omitempty"`
	Body      []byte        `json:"-"`
}
```

Update the local `ingressRequestJSON` struct and marshal construction in the same file to include `Subpath` but not `Body`:

```go
type ingressRequestJSON struct {
	RequestID string        `json:"request_id"`
	Endpoint  EndpointKind  `json:"endpoint"`
	Platform  Platform      `json:"platform"`
	Transport TransportKind `json:"transport"`
	Method    string        `json:"method"`
	Path      string        `json:"path"`
	Subpath   string        `json:"subpath,omitempty"`
	Header    http.Header   `json:"header,omitempty"`
}
```

Modify `backend/internal/gateway/domain/identity.go`:

```go
type GroupSnapshot struct {
	ID                 int64    `json:"id"`
	Name               string   `json:"name,omitempty"`
	Platform           Platform `json:"platform,omitempty"`
	DefaultMappedModel string   `json:"default_mapped_model,omitempty"`
}
```

- [ ] **Step 4: Expand import-boundary tests**

Modify the loop in `backend/internal/gateway/import_boundary_test.go` so foundation and new gateway-owned packages are checked:

```go
for _, packageDir := range []string{"domain", "core", "provider", "planning", "scheduler"} {
```

Add an allowed edge-package check:

```go
func TestGatewayEdgePackagesMayImportLegacyOnlyAtEdges(t *testing.T) {
	edgePackages := map[string]bool{
		"ingress":  true,
		"adapters": true,
	}
	for packageDir := range edgePackages {
		if _, err := os.Stat(packageDir); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("stat package dir %s: %v", packageDir, err)
		}
	}
}
```

- [ ] **Step 5: Run domain and boundary tests**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/gateway/domain ./internal/gateway
```

Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/gateway/domain/routing_plan.go backend/internal/gateway/domain/identity.go backend/internal/gateway/domain/routing_plan_test.go backend/internal/gateway/import_boundary_test.go
git commit -m "feat: extend gateway domain for responses ingress"
```

## Task 2: OpenAI Responses Ingress Adapter

**Files:**
- Create: `backend/internal/gateway/ingress/openai_responses.go`
- Create: `backend/internal/gateway/ingress/openai_responses_test.go`

- [ ] **Step 1: Write failing ingress tests**

Create `backend/internal/gateway/ingress/openai_responses_test.go`:

```go
package ingress

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	"github.com/stretchr/testify/require"
)

func TestBuildOpenAIResponsesIngressHTTPAliasAndSubpath(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses/compact/detail", nil)
	req.Header.Set("X-Request-Id", "req-123")
	req.Header.Set("Authorization", "Bearer secret")

	got, err := BuildOpenAIResponses(OpenAIResponsesInput{
		Request:   req,
		Body:      []byte(`{"model":"gpt-5.1"}`),
		Transport: domain.TransportHTTP,
		Subject: domain.Subject{
			APIKey: domain.APIKeySnapshot{ID: 7, UserID: 11, GroupID: 13},
			User:   domain.UserSnapshot{ID: 11},
			Group:  domain.GroupSnapshot{ID: 13, Platform: domain.PlatformOpenAI},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "req-123", got.RequestID)
	require.Equal(t, domain.EndpointOpenAIResponses, got.Endpoint)
	require.Equal(t, domain.PlatformOpenAI, got.Platform)
	require.Equal(t, domain.TransportHTTP, got.Transport)
	require.Equal(t, http.MethodPost, got.Method)
	require.Equal(t, "/openai/v1/responses/compact/detail", got.Path)
	require.Equal(t, "/compact/detail", got.Subpath)
	require.Equal(t, []byte(`{"model":"gpt-5.1"}`), got.Body)
}

func TestBuildOpenAIResponsesIngressWebSocket(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	got, err := BuildOpenAIResponses(OpenAIResponsesInput{
		Request:   req,
		Transport: domain.TransportWebSocket,
		Subject: domain.Subject{
			APIKey: domain.APIKeySnapshot{ID: 1, UserID: 2, GroupID: 3},
			User:   domain.UserSnapshot{ID: 2},
			Group:  domain.GroupSnapshot{ID: 3, Platform: domain.PlatformOpenAI},
		},
	})
	require.NoError(t, err)
	require.Equal(t, domain.TransportWebSocket, got.Transport)
	require.Equal(t, http.MethodGet, got.Method)
	require.Equal(t, "", got.Subpath)
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/gateway/ingress
```

Expected: fail because package and `BuildOpenAIResponses` do not exist.

- [ ] **Step 3: Implement ingress adapter**

Create `backend/internal/gateway/ingress/openai_responses.go`:

```go
package ingress

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	"github.com/google/uuid"
)

type OpenAIResponsesInput struct {
	Request   *http.Request
	Body      []byte
	Transport domain.TransportKind
	Subject   domain.Subject
}

func BuildOpenAIResponses(input OpenAIResponsesInput) (domain.IngressRequest, error) {
	if input.Request == nil || input.Request.URL == nil {
		return domain.IngressRequest{}, errors.New("request is required")
	}
	transport := input.Transport
	if transport == "" {
		transport = domain.TransportHTTP
	}
	return domain.IngressRequest{
		RequestID: requestID(input.Request),
		Endpoint:  domain.EndpointOpenAIResponses,
		Platform:  domain.PlatformOpenAI,
		Transport: transport,
		Method:    input.Request.Method,
		Path:      input.Request.URL.Path,
		Subpath:   responsesSubpath(input.Request.URL.Path),
		Header:    input.Request.Header.Clone(),
		Body:      append([]byte(nil), input.Body...),
	}, nil
}

func requestID(req *http.Request) string {
	for _, name := range []string{"X-Request-Id", "X-Request-ID", "X-Codex-Request-Id"} {
		if value := strings.TrimSpace(req.Header.Get(name)); value != "" {
			return value
		}
	}
	return uuid.NewString()
}

func responsesSubpath(path string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(path), "/")
	idx := strings.LastIndex(trimmed, "/responses")
	if idx < 0 {
		return ""
	}
	suffix := trimmed[idx+len("/responses"):]
	if suffix == "/" {
		return ""
	}
	return suffix
}
```

- [ ] **Step 4: Run ingress tests**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/gateway/ingress
```

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/gateway/ingress/openai_responses.go backend/internal/gateway/ingress/openai_responses_test.go
git commit -m "feat: add OpenAI responses ingress adapter"
```

## Task 3: OpenAI Responses Provider Parser

**Files:**
- Create: `backend/internal/gateway/provider/openai/responses.go`
- Create: `backend/internal/gateway/provider/openai/responses_test.go`

- [ ] **Step 1: Write failing parser tests**

Create `backend/internal/gateway/provider/openai/responses_test.go`:

```go
package openai

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	"github.com/stretchr/testify/require"
)

func TestParseResponsesValidHTTP(t *testing.T) {
	parsed, err := ParseResponses(domain.IngressRequest{
		Endpoint:  domain.EndpointOpenAIResponses,
		Transport: domain.TransportHTTP,
		Path:      "/v1/responses",
		Body:      []byte(`{"model":"gpt-5.1","stream":true,"previous_response_id":"resp_123","service_tier":"priority"}`),
		Header:    http.Header{"Session-Id": []string{"session-1"}},
	})
	require.NoError(t, err)
	require.Equal(t, "gpt-5.1", parsed.Canonical.RequestedModel)
	require.True(t, parsed.Stream)
	require.Equal(t, "resp_123", parsed.PreviousResponseID)
	require.Equal(t, PreviousResponseKindResponse, parsed.PreviousResponseKind)
	require.Equal(t, domain.SessionSourceHeader, parsed.Canonical.Session.Source)
	require.Equal(t, "session-1", parsed.Canonical.Session.Key)
	require.Equal(t, "priority", parsed.ServiceTier)
}

func TestParseResponsesCompactNormalizesBody(t *testing.T) {
	parsed, err := ParseResponses(domain.IngressRequest{
		Endpoint:  domain.EndpointOpenAIResponses,
		Transport: domain.TransportHTTP,
		Path:      "/responses/compact",
		Subpath:   "/compact",
		Body: []byte(`{
			"model":{"value":"gpt-5.1"},
			"input":{"value":"hello"},
			"prompt_cache_key":"compact-seed"
		}`),
	})
	require.NoError(t, err)
	require.True(t, parsed.Compact)
	require.Equal(t, "compact-seed", parsed.CompactSessionSeed)
	require.JSONEq(t, `{"input":"hello","model":"gpt-5.1","prompt_cache_key":"compact-seed"}`, string(parsed.NormalizedBody))
}

func TestParseResponsesRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "empty", body: "", want: "Request body is empty"},
		{name: "invalid json", body: "{", want: "Failed to parse request body"},
		{name: "missing model", body: `{}`, want: "model is required"},
		{name: "non string model", body: `{"model":123}`, want: "model is required"},
		{name: "invalid stream", body: `{"model":"gpt-5.1","stream":"yes"}`, want: "invalid stream field type"},
		{name: "message previous id", body: `{"model":"gpt-5.1","previous_response_id":"msg_123"}`, want: "previous_response_id must be a response.id (resp_*), not a message id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseResponses(domain.IngressRequest{Body: []byte(tt.body)})
			require.Error(t, err)
			require.ErrorContains(t, err, tt.want)
		})
	}
}
```

- [ ] **Step 2: Run parser tests to verify failure**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/gateway/provider/openai
```

Expected: fail because package and parser do not exist.

- [ ] **Step 3: Implement parser**

Create `backend/internal/gateway/provider/openai/responses.go`:

```go
package openai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	"github.com/tidwall/gjson"
)

type PreviousResponseKind string

const (
	PreviousResponseKindNone     PreviousResponseKind = "none"
	PreviousResponseKindResponse PreviousResponseKind = "response"
	PreviousResponseKindMessage  PreviousResponseKind = "message"
	PreviousResponseKindOther    PreviousResponseKind = "other"
)

type ResponsesParseResult struct {
	Canonical          domain.CanonicalRequest
	NormalizedBody      []byte
	Stream              bool
	Compact             bool
	CompactSessionSeed  string
	PreviousResponseID  string
	PreviousResponseKind PreviousResponseKind
	ServiceTier         string
	ReasoningEffort     string
	BodySHA256          string
}

func ParseResponses(req domain.IngressRequest) (ResponsesParseResult, error) {
	body := append([]byte(nil), req.Body...)
	if len(body) == 0 {
		return ResponsesParseResult{}, errors.New("Request body is empty")
	}
	if !gjson.ValidBytes(body) {
		return ResponsesParseResult{}, errors.New("Failed to parse request body")
	}

	compact := isCompactSubpath(req.Subpath)
	compactSeed := ""
	if compact {
		seed, normalized, changed, err := normalizeCompactBody(body)
		if err != nil {
			return ResponsesParseResult{}, fmt.Errorf("Failed to normalize compact request body: %w", err)
		}
		compactSeed = seed
		if changed {
			body = normalized
		}
	}

	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String || strings.TrimSpace(modelResult.String()) == "" {
		return ResponsesParseResult{}, errors.New("model is required")
	}
	streamResult := gjson.GetBytes(body, "stream")
	if streamResult.Exists() && streamResult.Type != gjson.True && streamResult.Type != gjson.False {
		return ResponsesParseResult{}, errors.New("invalid stream field type")
	}

	previousID := strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String())
	previousKind := classifyPreviousResponseID(previousID)
	if previousKind == PreviousResponseKindMessage {
		return ResponsesParseResult{}, errors.New("previous_response_id must be a response.id (resp_*), not a message id")
	}

	reasoningEffort := strings.TrimSpace(gjson.GetBytes(body, "reasoning.effort").String())
	serviceTier := strings.TrimSpace(gjson.GetBytes(body, "service_tier").String())
	hash := sha256.Sum256(body)

	return ResponsesParseResult{
		Canonical: domain.CanonicalRequest{
			RequestedModel: modelResult.String(),
			Headers:        req.Header.Clone(),
			Body:           append([]byte(nil), body...),
			Model: domain.ModelResolution{
				Requested: modelResult.String(),
				Canonical: modelResult.String(),
			},
			Session: resolveSession(req.Header, compactSeed, previousID),
		},
		NormalizedBody:      body,
		Stream:              streamResult.Bool(),
		Compact:             compact,
		CompactSessionSeed:  compactSeed,
		PreviousResponseID:  previousID,
		PreviousResponseKind: previousKind,
		ServiceTier:         serviceTier,
		ReasoningEffort:     reasoningEffort,
		BodySHA256:          hex.EncodeToString(hash[:]),
	}, nil
}

func isCompactSubpath(subpath string) bool {
	trimmed := strings.Trim(strings.TrimSpace(subpath), "/")
	return trimmed == "compact" || strings.HasPrefix(trimmed, "compact/")
}

func normalizeCompactBody(body []byte) (string, []byte, bool, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", body, false, err
	}
	changed := false
	for _, field := range []string{"model", "input", "instructions", "previous_response_id"} {
		if value, ok := raw[field].(map[string]any); ok {
			if inner, exists := value["value"]; exists {
				raw[field] = inner
				changed = true
			}
		}
	}
	seed, _ := raw["prompt_cache_key"].(string)
	if !changed {
		return strings.TrimSpace(seed), body, false, nil
	}
	normalized, err := json.Marshal(raw)
	if err != nil {
		return "", body, false, err
	}
	return strings.TrimSpace(seed), normalized, true, nil
}

func classifyPreviousResponseID(value string) PreviousResponseKind {
	trimmed := strings.TrimSpace(value)
	switch {
	case trimmed == "":
		return PreviousResponseKindNone
	case strings.HasPrefix(trimmed, "resp_"):
		return PreviousResponseKindResponse
	case strings.HasPrefix(trimmed, "msg_"):
		return PreviousResponseKindMessage
	default:
		return PreviousResponseKindOther
	}
}

func resolveSession(headers http.Header, compactSeed, previousID string) domain.SessionInput {
	if value := strings.TrimSpace(headers.Get("session_id")); value != "" {
		return domain.SessionInput{Key: value, Source: domain.SessionSourceHeader}
	}
	if value := strings.TrimSpace(headers.Get("conversation_id")); value != "" {
		return domain.SessionInput{Key: value, Source: domain.SessionSourceHeader}
	}
	if compactSeed != "" {
		return domain.SessionInput{Key: compactSeed, Source: domain.SessionSourcePromptCacheKey}
	}
	if previousID != "" {
		return domain.SessionInput{Key: previousID, Source: domain.SessionSourcePreviousResponseID}
	}
	return domain.SessionInput{Source: domain.SessionSourceNone}
}
```

- [ ] **Step 4: Run parser tests**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/gateway/provider/openai
```

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/gateway/provider/openai/responses.go backend/internal/gateway/provider/openai/responses_test.go
git commit -m "feat: parse OpenAI responses gateway requests"
```

## Task 4: OpenAI Responses RoutingPlan Builder

**Files:**
- Create: `backend/internal/gateway/planning/openai_responses.go`
- Create: `backend/internal/gateway/planning/openai_responses_test.go`

- [ ] **Step 1: Write failing plan fixture tests**

Create `backend/internal/gateway/planning/openai_responses_test.go`:

```go
package planning

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	openai "github.com/Wei-Shaw/sub2api/internal/gateway/provider/openai"
	"github.com/stretchr/testify/require"
)

func TestBuildOpenAIResponsesPlanHTTPFixture(t *testing.T) {
	createdAt := time.Date(2026, 4, 26, 8, 0, 0, 0, time.UTC)
	plan := BuildOpenAIResponsesPlan(OpenAIResponsesPlanInput{
		Ingress: domain.IngressRequest{
			RequestID: "req-plan",
			Endpoint:  domain.EndpointOpenAIResponses,
			Platform:  domain.PlatformOpenAI,
			Transport: domain.TransportHTTP,
			Method:    http.MethodPost,
			Path:      "/v1/responses",
		},
		Subject: domain.Subject{
			APIKey: domain.APIKeySnapshot{ID: 7, UserID: 11, GroupID: 13},
			User:   domain.UserSnapshot{ID: 11},
			Group:  domain.GroupSnapshot{ID: 13, Platform: domain.PlatformOpenAI},
		},
		Parsed: openai.ResponsesParseResult{
			Canonical: domain.CanonicalRequest{
				RequestedModel: "gpt-5.1",
				Model:          domain.ModelResolution{Requested: "gpt-5.1", Canonical: "gpt-5.1"},
				Session:        domain.SessionInput{Key: "session-1", Source: domain.SessionSourceHeader},
			},
			NormalizedBody: []byte(`{"model":"gpt-5.1","stream":true}`),
			Stream:         true,
			BodySHA256:     "abc123",
		},
		MaxAccountSwitches: 3,
		CreatedAt:         createdAt,
	})

	raw, err := json.Marshal(plan)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"request":{"request_id":"req-plan","endpoint":"openai_responses","platform":"openai","transport":"http","method":"POST","path":"/v1/responses"},
		"subject":{"api_key":{"id":7,"user_id":11,"group_id":13,"policy":{"group_id":0,"rate_limit":{}}},"user":{"id":11},"group":{"id":13,"platform":"openai"}},
		"canonical":{"requested_model":"gpt-5.1","model":{"requested":"gpt-5.1","canonical":"gpt-5.1"},"session":{"key":"session-1","source":"header"},"mutation":{}},
		"group_id":13,
		"session":{"enabled":true,"key":"session-1","source":"header","sticky":true},
		"diagnostics":{"total":0,"eligible":0,"rejected":0},
		"account":{"layer":"","account":{"id":0,"platform":"","type":"","capabilities":{}},"reservation":{"account_id":0,"expires_at":"0001-01-01T00:00:00Z"},"wait_plan":{"required":false,"timeout":0}},
		"retry":{"max_attempts":4,"retry_same_account":true,"retry_other_accounts":true},
		"billing":{"mode":"streaming","events":["reserve","finalize","release"]},
		"debug":{"enabled":true,"body_fingerprint":{"sha256":"abc123","bytes":35}},
		"created_at":"2026-04-26T08:00:00Z"
	}`, string(raw))
}
```

- [ ] **Step 2: Run plan tests to verify failure**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/gateway/planning
```

Expected: fail because package and builder do not exist.

- [ ] **Step 3: Implement plan builder**

Create `backend/internal/gateway/planning/openai_responses.go`:

```go
package planning

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	openai "github.com/Wei-Shaw/sub2api/internal/gateway/provider/openai"
)

type OpenAIResponsesPlanInput struct {
	Ingress            domain.IngressRequest
	Subject            domain.Subject
	Parsed             openai.ResponsesParseResult
	MaxAccountSwitches int
	CreatedAt          time.Time
}

func BuildOpenAIResponsesPlan(input OpenAIResponsesPlanInput) domain.RoutingPlan {
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	maxAttempts := input.MaxAccountSwitches + 1
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	billing := domain.BillingLifecyclePlan{
		Mode:  domain.BillingModeToken,
		Model: input.Parsed.Canonical.RequestedModel,
		Events: []domain.BillingEventKind{
			domain.BillingEventCharge,
		},
	}
	if input.Parsed.Stream || input.Ingress.Transport == domain.TransportWebSocket {
		billing.Mode = domain.BillingModeStreaming
		billing.Events = []domain.BillingEventKind{
			domain.BillingEventReserve,
			domain.BillingEventFinalize,
			domain.BillingEventRelease,
		}
	}
	session := domain.SessionDecision{
		Enabled: input.Parsed.Canonical.Session.Source != domain.SessionSourceNone,
		Key:     input.Parsed.Canonical.Session.Key,
		Source:  input.Parsed.Canonical.Session.Source,
		Sticky:  input.Parsed.Canonical.Session.Key != "",
	}
	return domain.RoutingPlan{
		Request:   input.Ingress,
		Subject:   input.Subject,
		Canonical: input.Parsed.Canonical,
		GroupID:   input.Subject.APIKey.GroupID,
		Session:   session,
		Retry: domain.RetryPlan{
			MaxAttempts:        maxAttempts,
			RetrySameAccount:   true,
			RetryOtherAccounts: true,
		},
		Billing: billing,
		Debug: domain.DebugPlan{
			Enabled: true,
			BodyFingerprint: domain.BodyFingerprint{
				SHA256: input.Parsed.BodySHA256,
				Bytes:  int64(len(input.Parsed.NormalizedBody)),
			},
		},
		CreatedAt: createdAt,
	}
}
```

- [ ] **Step 4: Run plan tests**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/gateway/planning
```

Expected: pass. If JSON byte count differs because of compacted body content, update only the expected `bytes` number to match `len([]byte(...))`; do not weaken field assertions.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/gateway/planning/openai_responses.go backend/internal/gateway/planning/openai_responses_test.go
git commit -m "feat: build OpenAI responses routing plans"
```

## Task 5: Native Scheduler Types And Core Algorithm

**Files:**
- Create: `backend/internal/gateway/scheduler/openai.go`
- Create: `backend/internal/gateway/scheduler/openai_test.go`

- [ ] **Step 1: Write failing scheduler tests**

Create `backend/internal/gateway/scheduler/openai_test.go`:

```go
package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	"github.com/stretchr/testify/require"
)

func TestOpenAISchedulerPreviousResponseStickyWins(t *testing.T) {
	ports := newFakePorts()
	ports.accounts[10] = testAccount(10, "oauth", "gpt-5.1", domain.TransportWebSocket)
	ports.previous["resp_1"] = 10

	s := NewOpenAIScheduler(ports)
	result, err := s.Select(context.Background(), ScheduleRequest{
		GroupID:            1,
		RequestedModel:     "gpt-5.1",
		PreviousResponseID: "resp_1",
		RequiredTransport:  domain.TransportWebSocket,
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), result.Account.ID)
	require.Equal(t, domain.AccountDecisionPreviousResponseID, result.Layer)
	require.True(t, result.Reservation.Acquired)
}

func TestOpenAISchedulerSessionStickyFallsBackWhenTransportMismatch(t *testing.T) {
	ports := newFakePorts()
	ports.accounts[10] = testAccount(10, "oauth", "gpt-5.1", domain.TransportHTTP)
	ports.accounts[20] = testAccount(20, "oauth", "gpt-5.1", domain.TransportWebSocket)
	ports.sticky["group:1:session-1"] = 10

	s := NewOpenAIScheduler(ports)
	result, err := s.Select(context.Background(), ScheduleRequest{
		GroupID:           1,
		SessionKey:        "session-1",
		RequestedModel:    "gpt-5.1",
		RequiredTransport: domain.TransportWebSocket,
	})
	require.NoError(t, err)
	require.Equal(t, int64(20), result.Account.ID)
	require.Equal(t, domain.AccountDecisionLoadBalance, result.Layer)
	require.Equal(t, 1, result.Diagnostics.RejectCount[domain.RejectionReasonTransportMismatch])
}

func TestOpenAISchedulerLoadBalanceExcludesFailedAccounts(t *testing.T) {
	ports := newFakePorts()
	ports.accounts[10] = testAccount(10, "api_key", "gpt-5.1", domain.TransportHTTP)
	ports.accounts[20] = testAccount(20, "api_key", "gpt-5.1", domain.TransportHTTP)

	s := NewOpenAIScheduler(ports)
	result, err := s.Select(context.Background(), ScheduleRequest{
		GroupID:           1,
		RequestedModel:    "gpt-5.1",
		RequiredTransport: domain.TransportHTTP,
		ExcludedAccountIDs: map[int64]struct{}{
			10: {},
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(20), result.Account.ID)
	require.Equal(t, 1, result.Diagnostics.RejectCount[domain.RejectionReasonExcluded])
}

func TestOpenAISchedulerNoAccountReturnsDiagnostics(t *testing.T) {
	ports := newFakePorts()
	ports.accounts[10] = testAccount(10, "api_key", "gpt-4o", domain.TransportHTTP)

	s := NewOpenAIScheduler(ports)
	_, err := s.Select(context.Background(), ScheduleRequest{
		GroupID:           1,
		RequestedModel:    "gpt-5.1",
		RequiredTransport: domain.TransportHTTP,
	})
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
	var scheduleErr *NoAvailableAccountsError
	require.True(t, errors.As(err, &scheduleErr))
	require.Equal(t, 1, scheduleErr.Diagnostics.RejectCount[domain.RejectionReasonModelUnsupported])
}

func TestOpenAISchedulerWaitPlanWhenReservationBusy(t *testing.T) {
	ports := newFakePorts()
	ports.accounts[10] = testAccount(10, "api_key", "gpt-5.1", domain.TransportHTTP)
	ports.busy[10] = true

	s := NewOpenAIScheduler(ports)
	result, err := s.Select(context.Background(), ScheduleRequest{
		GroupID:           1,
		RequestedModel:    "gpt-5.1",
		RequiredTransport: domain.TransportHTTP,
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), result.Account.ID)
	require.False(t, result.Reservation.Acquired)
	require.True(t, result.WaitPlan.Required)
}
```

Add fake helpers in the same test file:

```go
type fakePorts struct {
	accounts map[int64]Account
	previous map[string]int64
	sticky   map[string]int64
	busy     map[int64]bool
}

func newFakePorts() *fakePorts {
	return &fakePorts{
		accounts: map[int64]Account{},
		previous: map[string]int64{},
		sticky:   map[string]int64{},
		busy:     map[int64]bool{},
	}
}

func testAccount(id int64, accountType string, model string, transports ...domain.TransportKind) Account {
	return Account{
		Snapshot: domain.AccountSnapshot{
			ID:          id,
			Platform:    domain.PlatformOpenAI,
			Type:        domain.AccountType(accountType),
			Priority:    int(id),
			Concurrency: 1,
			Capabilities: domain.AccountCapabilities{
				Models:     []string{model},
				Transports: transports,
				Streaming:  true,
			},
		},
	}
}
```

- [ ] **Step 2: Run scheduler tests to verify failure**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/gateway/scheduler
```

Expected: fail because package and scheduler types do not exist.

- [ ] **Step 3: Implement scheduler**

Create `backend/internal/gateway/scheduler/openai.go`:

```go
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
)

var ErrNoAvailableAccounts = errors.New("no available accounts")

type Account struct {
	Snapshot domain.AccountSnapshot
	Legacy   any
}

type ScheduleRequest struct {
	GroupID            int64
	SessionKey         string
	PreviousResponseID string
	RequestedModel     string
	RequiredTransport  domain.TransportKind
	ExcludedAccountIDs map[int64]struct{}
}

type ScheduleResult struct {
	Account     Account
	Layer       domain.AccountDecisionLayer
	Reservation Reservation
	WaitPlan    domain.AccountWaitPlan
	Diagnostics domain.CandidateDiagnostics
}

type Reservation struct {
	AccountID int64
	Acquired  bool
	Release   func()
}

type ScheduleOutcome struct {
	Success      bool
	FirstTokenMs *int
}

type Ports interface {
	LookupPreviousResponseAccount(ctx context.Context, groupID int64, previousResponseID string) (int64, bool, error)
	GetStickySessionAccount(ctx context.Context, groupID int64, sessionKey string) (int64, bool, error)
	BindStickySession(ctx context.Context, groupID int64, sessionKey string, accountID int64) error
	DeleteStickySession(ctx context.Context, groupID int64, sessionKey string) error
	RefreshStickySession(ctx context.Context, groupID int64, sessionKey string) error
	ListSchedulableOpenAIAccounts(ctx context.Context, groupID int64) ([]Account, error)
	GetAccount(ctx context.Context, accountID int64) (Account, bool, error)
	AcquireAccountSlot(ctx context.Context, account Account) (Reservation, error)
	WaitPlan(ctx context.Context, account Account) domain.AccountWaitPlan
	ReportResult(ctx context.Context, accountID int64, outcome ScheduleOutcome)
}

type OpenAIScheduler struct {
	ports Ports
}

func NewOpenAIScheduler(ports Ports) *OpenAIScheduler {
	return &OpenAIScheduler{ports: ports}
}

type NoAvailableAccountsError struct {
	Diagnostics domain.CandidateDiagnostics
}

func (e *NoAvailableAccountsError) Error() string {
	return ErrNoAvailableAccounts.Error()
}

func (e *NoAvailableAccountsError) Unwrap() error {
	return ErrNoAvailableAccounts
}

func (s *OpenAIScheduler) Select(ctx context.Context, req ScheduleRequest) (*ScheduleResult, error) {
	if s == nil || s.ports == nil {
		return nil, fmt.Errorf("%w: scheduler ports unavailable", ErrNoAvailableAccounts)
	}
	if req.RequiredTransport == "" {
		req.RequiredTransport = domain.TransportHTTP
	}
	diagnostics := domain.CandidateDiagnostics{RejectCount: map[domain.RejectionReason]int{}}

	if strings.TrimSpace(req.PreviousResponseID) != "" {
		if accountID, ok, err := s.ports.LookupPreviousResponseAccount(ctx, req.GroupID, req.PreviousResponseID); err != nil {
			return nil, err
		} else if ok {
			if result, ok, err := s.trySpecificAccount(ctx, req, accountID, domain.AccountDecisionPreviousResponseID, &diagnostics); err != nil {
				return nil, err
			} else if ok {
				return result, nil
			}
		}
	}

	if strings.TrimSpace(req.SessionKey) != "" {
		if accountID, ok, err := s.ports.GetStickySessionAccount(ctx, req.GroupID, req.SessionKey); err != nil {
			return nil, err
		} else if ok {
			if result, ok, err := s.trySpecificAccount(ctx, req, accountID, domain.AccountDecisionSessionHash, &diagnostics); err != nil {
				return nil, err
			} else if ok {
				return result, nil
			}
			_ = s.ports.DeleteStickySession(ctx, req.GroupID, req.SessionKey)
		}
	}

	accounts, err := s.ports.ListSchedulableOpenAIAccounts(ctx, req.GroupID)
	if err != nil {
		return nil, err
	}
	diagnostics.Total += len(accounts)
	sort.SliceStable(accounts, func(i, j int) bool {
		if accounts[i].Snapshot.Priority != accounts[j].Snapshot.Priority {
			return accounts[i].Snapshot.Priority < accounts[j].Snapshot.Priority
		}
		return accounts[i].Snapshot.ID < accounts[j].Snapshot.ID
	})
	for _, account := range accounts {
		if !eligible(account, req, &diagnostics) {
			continue
		}
		return s.reserve(ctx, req, account, domain.AccountDecisionLoadBalance, diagnostics)
	}
	diagnostics.Rejected = sumRejects(diagnostics.RejectCount)
	return nil, &NoAvailableAccountsError{Diagnostics: diagnostics}
}

func (s *OpenAIScheduler) ReportResult(ctx context.Context, accountID int64, outcome ScheduleOutcome) {
	if s == nil || s.ports == nil {
		return
	}
	s.ports.ReportResult(ctx, accountID, outcome)
}

func (s *OpenAIScheduler) trySpecificAccount(ctx context.Context, req ScheduleRequest, accountID int64, layer domain.AccountDecisionLayer, diagnostics *domain.CandidateDiagnostics) (*ScheduleResult, bool, error) {
	account, ok, err := s.ports.GetAccount(ctx, accountID)
	if err != nil || !ok {
		return nil, false, err
	}
	diagnostics.Total++
	if !eligible(account, req, diagnostics) {
		return nil, false, nil
	}
	result, err := s.reserve(ctx, req, account, layer, *diagnostics)
	if err != nil {
		return nil, false, err
	}
	return result, true, nil
}

func (s *OpenAIScheduler) reserve(ctx context.Context, req ScheduleRequest, account Account, layer domain.AccountDecisionLayer, diagnostics domain.CandidateDiagnostics) (*ScheduleResult, error) {
	reservation, err := s.ports.AcquireAccountSlot(ctx, account)
	if err != nil {
		return nil, err
	}
	if reservation.Acquired && strings.TrimSpace(req.SessionKey) != "" {
		_ = s.ports.BindStickySession(ctx, req.GroupID, req.SessionKey, account.Snapshot.ID)
	}
	waitPlan := domain.AccountWaitPlan{}
	if !reservation.Acquired {
		waitPlan = s.ports.WaitPlan(ctx, account)
		if !waitPlan.Required {
			waitPlan.Required = true
			waitPlan.Reason = "account_busy"
			waitPlan.Timeout = 30 * time.Second
		}
	}
	diagnostics.Eligible++
	diagnostics.Rejected = sumRejects(diagnostics.RejectCount)
	return &ScheduleResult{
		Account:     account,
		Layer:       layer,
		Reservation: reservation,
		WaitPlan:    waitPlan,
		Diagnostics: diagnostics,
	}, nil
}

func eligible(account Account, req ScheduleRequest, diagnostics *domain.CandidateDiagnostics) bool {
	if _, excluded := req.ExcludedAccountIDs[account.Snapshot.ID]; excluded {
		reject(diagnostics, account.Snapshot.ID, domain.RejectionReasonExcluded)
		return false
	}
	if account.Snapshot.Platform != domain.PlatformOpenAI {
		reject(diagnostics, account.Snapshot.ID, domain.RejectionReasonPlatformMismatch)
		return false
	}
	if req.RequestedModel != "" && !supportsModel(account.Snapshot.Capabilities.Models, req.RequestedModel) {
		reject(diagnostics, account.Snapshot.ID, domain.RejectionReasonModelUnsupported)
		return false
	}
	if !supportsTransport(account.Snapshot.Capabilities.Transports, req.RequiredTransport) {
		reject(diagnostics, account.Snapshot.ID, domain.RejectionReasonTransportMismatch)
		return false
	}
	return true
}

func supportsModel(models []string, requested string) bool {
	if len(models) == 0 {
		return true
	}
	for _, model := range models {
		if model == requested {
			return true
		}
	}
	return false
}

func supportsTransport(transports []domain.TransportKind, required domain.TransportKind) bool {
	if required == "" || required == domain.TransportHTTP || len(transports) == 0 {
		return true
	}
	for _, transport := range transports {
		if transport == required {
			return true
		}
	}
	return false
}

func reject(diagnostics *domain.CandidateDiagnostics, accountID int64, reason domain.RejectionReason) {
	if diagnostics.RejectCount == nil {
		diagnostics.RejectCount = map[domain.RejectionReason]int{}
	}
	diagnostics.RejectCount[reason]++
	diagnostics.Samples = append(diagnostics.Samples, domain.CandidateSample{AccountID: accountID, Reason: reason})
}

func sumRejects(values map[domain.RejectionReason]int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}
```

Add fake port methods to `openai_test.go`:

```go
func (p *fakePorts) LookupPreviousResponseAccount(ctx context.Context, groupID int64, previousResponseID string) (int64, bool, error) {
	id, ok := p.previous[previousResponseID]
	return id, ok, nil
}

func (p *fakePorts) GetStickySessionAccount(ctx context.Context, groupID int64, sessionKey string) (int64, bool, error) {
	id, ok := p.sticky[fmt.Sprintf("group:%d:%s", groupID, sessionKey)]
	return id, ok, nil
}

func (p *fakePorts) BindStickySession(ctx context.Context, groupID int64, sessionKey string, accountID int64) error {
	p.sticky[fmt.Sprintf("group:%d:%s", groupID, sessionKey)] = accountID
	return nil
}

func (p *fakePorts) DeleteStickySession(ctx context.Context, groupID int64, sessionKey string) error {
	delete(p.sticky, fmt.Sprintf("group:%d:%s", groupID, sessionKey))
	return nil
}

func (p *fakePorts) RefreshStickySession(ctx context.Context, groupID int64, sessionKey string) error {
	return nil
}

func (p *fakePorts) ListSchedulableOpenAIAccounts(ctx context.Context, groupID int64) ([]Account, error) {
	out := make([]Account, 0, len(p.accounts))
	for _, account := range p.accounts {
		out = append(out, account)
	}
	return out, nil
}

func (p *fakePorts) GetAccount(ctx context.Context, accountID int64) (Account, bool, error) {
	account, ok := p.accounts[accountID]
	return account, ok, nil
}

func (p *fakePorts) AcquireAccountSlot(ctx context.Context, account Account) (Reservation, error) {
	if p.busy[account.Snapshot.ID] {
		return Reservation{AccountID: account.Snapshot.ID, Acquired: false}, nil
	}
	return Reservation{AccountID: account.Snapshot.ID, Acquired: true, Release: func() {}}, nil
}

func (p *fakePorts) WaitPlan(ctx context.Context, account Account) domain.AccountWaitPlan {
	return domain.AccountWaitPlan{Required: true, Reason: "account_busy", Timeout: time.Second}
}

func (p *fakePorts) ReportResult(ctx context.Context, accountID int64, outcome ScheduleOutcome) {}
```

Also add `fmt` to the test imports.

- [ ] **Step 4: Run scheduler tests**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/gateway/scheduler
```

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/gateway/scheduler/openai.go backend/internal/gateway/scheduler/openai_test.go
git commit -m "feat: add native OpenAI account scheduler"
```

## Task 6: Legacy Scheduler Adapter Bridge

**Files:**
- Create: `backend/internal/gateway/adapters/openai_scheduler.go`
- Create: `backend/internal/gateway/adapters/openai_scheduler_test.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/openai_account_scheduler.go`

- [ ] **Step 1: Write failing adapter mapping tests**

Create `backend/internal/gateway/adapters/openai_scheduler_test.go`:

```go
package adapters

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAccountToSchedulerAccountMapsTransportCapabilities(t *testing.T) {
	account := &service.Account{
		ID:          42,
		Name:        "oauth-ws",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Priority:    2,
		Concurrency: 3,
		Models:      []string{"gpt-5.1"},
	}

	got := OpenAIAccountToSchedulerAccount(account, []domain.TransportKind{domain.TransportHTTP, domain.TransportWebSocket})
	require.Equal(t, int64(42), got.Snapshot.ID)
	require.Equal(t, domain.PlatformOpenAI, got.Snapshot.Platform)
	require.Equal(t, domain.AccountTypeOAuth, got.Snapshot.Type)
	require.Equal(t, []string{"gpt-5.1"}, got.Snapshot.Capabilities.Models)
	require.Equal(t, account, got.Legacy)
}
```

- [ ] **Step 2: Run adapter tests to verify failure**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/gateway/adapters
```

Expected: fail because adapter does not exist.

- [ ] **Step 3: Implement adapter account mapping**

Create `backend/internal/gateway/adapters/openai_scheduler.go` with mapping helpers first:

```go
package adapters

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	"github.com/Wei-Shaw/sub2api/internal/gateway/scheduler"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type OpenAISchedulerBridge interface {
	GatewayListSchedulableOpenAIAccounts(ctx context.Context, groupID *int64) ([]service.Account, error)
	GatewayGetOpenAIAccount(ctx context.Context, accountID int64) (*service.Account, error)
	GatewayLookupPreviousResponseAccount(ctx context.Context, groupID *int64, previousResponseID string, requestedModel string, excluded map[int64]struct{}) (*service.Account, bool, error)
	GatewayGetStickySessionAccountID(ctx context.Context, groupID *int64, sessionKey string) (int64, bool, error)
	GatewayBindStickySession(ctx context.Context, groupID *int64, sessionKey string, accountID int64) error
	GatewayDeleteStickySession(ctx context.Context, groupID *int64, sessionKey string) error
	GatewayRefreshStickySession(ctx context.Context, groupID *int64, sessionKey string) error
	GatewayResolveOpenAITransports(account *service.Account) []domain.TransportKind
	GatewayAcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (func(), bool, error)
	GatewayDefaultAccountWaitPlan(accountID int64, maxConcurrency int) (timeout time.Duration, reason string)
	GatewayReportOpenAIAccountScheduleResult(accountID int64, success bool, firstTokenMs *int)
}

type OpenAISchedulerPorts struct {
	Bridge         OpenAISchedulerBridge
	GroupID        *int64
	RequestedModel string
	Excluded       map[int64]struct{}
}

func OpenAIAccountToSchedulerAccount(account *service.Account, transports []domain.TransportKind) scheduler.Account {
	if account == nil {
		return scheduler.Account{}
	}
	return scheduler.Account{
		Snapshot: domain.AccountSnapshot{
			ID:          account.ID,
			Name:        account.Name,
			Platform:    domain.Platform(account.Platform),
			Type:        domain.AccountType(account.Type),
			Priority:    account.Priority,
			Concurrency: account.Concurrency,
			Capabilities: domain.AccountCapabilities{
				Models:     append([]string(nil), account.Models...),
				Transports: append([]domain.TransportKind(nil), transports...),
				Streaming:  true,
			},
		},
		Legacy: account,
	}
}
```

- [ ] **Step 4: Run adapter mapping tests**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/gateway/adapters
```

Expected: pass.

- [ ] **Step 5: Add bridge methods on `OpenAIGatewayService`**

Add exported methods in `backend/internal/service/openai_account_scheduler.go` near `SelectAccountWithScheduler`:

```go
func (s *OpenAIGatewayService) GatewayListSchedulableOpenAIAccounts(ctx context.Context, groupID *int64) ([]Account, error) {
	return s.listSchedulableAccounts(ctx, groupID)
}

func (s *OpenAIGatewayService) GatewayGetOpenAIAccount(ctx context.Context, accountID int64) (*Account, error) {
	return s.getSchedulableAccount(ctx, accountID)
}

func (s *OpenAIGatewayService) GatewayLookupPreviousResponseAccount(ctx context.Context, groupID *int64, previousResponseID string, requestedModel string, excluded map[int64]struct{}) (*Account, bool, error) {
	selection, err := s.SelectAccountByPreviousResponseID(ctx, groupID, previousResponseID, requestedModel, excluded)
	if err != nil {
		return nil, false, err
	}
	if selection == nil || selection.Account == nil {
		return nil, false, nil
	}
	return selection.Account, true, nil
}

func (s *OpenAIGatewayService) GatewayGetStickySessionAccountID(ctx context.Context, groupID *int64, sessionKey string) (int64, bool, error) {
	accountID, err := s.getStickySessionAccountID(ctx, groupID, sessionKey)
	if err != nil || accountID <= 0 {
		return 0, false, err
	}
	return accountID, true, nil
}

func (s *OpenAIGatewayService) GatewayBindStickySession(ctx context.Context, groupID *int64, sessionKey string, accountID int64) error {
	return s.BindStickySession(ctx, groupID, sessionKey, accountID)
}

func (s *OpenAIGatewayService) GatewayDeleteStickySession(ctx context.Context, groupID *int64, sessionKey string) error {
	return s.deleteStickySessionAccountID(ctx, groupID, sessionKey)
}

func (s *OpenAIGatewayService) GatewayRefreshStickySession(ctx context.Context, groupID *int64, sessionKey string) error {
	return s.refreshStickySessionTTL(ctx, groupID, sessionKey, s.openAIWSSessionStickyTTL())
}

func (s *OpenAIGatewayService) GatewayResolveOpenAITransports(account *Account) []domain.TransportKind {
	transports := []domain.TransportKind{domain.TransportHTTP}
	if s != nil && account != nil &&
		s.getOpenAIWSProtocolResolver().Resolve(account).Transport == OpenAIUpstreamTransportResponsesWebsocketV2 {
		transports = append(transports, domain.TransportWebSocket)
	}
	return transports
}
```

Add account slot and reporting bridge methods:

```go
func (s *OpenAIGatewayService) GatewayAcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (func(), bool, error) {
	result, err := s.tryAcquireAccountSlot(ctx, accountID, maxConcurrency)
	if err != nil {
		return nil, false, err
	}
	if result == nil {
		return nil, false, nil
	}
	return result.ReleaseFunc, result.Acquired, nil
}

func (s *OpenAIGatewayService) GatewayDefaultAccountWaitPlan(accountID int64, maxConcurrency int) (time.Duration, string) {
	cfg := s.schedulingConfig()
	return cfg.FallbackWaitTimeout, "account_busy"
}
```

Use existing `ReportOpenAIAccountScheduleResult` directly for the interface method.

- [ ] **Step 6: Implement full adapter ports**

Append to `backend/internal/gateway/adapters/openai_scheduler.go`:

```go
func (p OpenAISchedulerPorts) LookupPreviousResponseAccount(ctx context.Context, groupID int64, previousResponseID string) (int64, bool, error) {
	account, ok, err := p.Bridge.GatewayLookupPreviousResponseAccount(ctx, p.GroupID, previousResponseID, p.RequestedModel, p.Excluded)
	if err != nil || !ok || account == nil {
		return 0, false, err
	}
	return account.ID, true, nil
}

func (p OpenAISchedulerPorts) GetStickySessionAccount(ctx context.Context, groupID int64, sessionKey string) (int64, bool, error) {
	return p.Bridge.GatewayGetStickySessionAccountID(ctx, p.GroupID, sessionKey)
}

func (p OpenAISchedulerPorts) BindStickySession(ctx context.Context, groupID int64, sessionKey string, accountID int64) error {
	return p.Bridge.GatewayBindStickySession(ctx, p.GroupID, sessionKey, accountID)
}

func (p OpenAISchedulerPorts) DeleteStickySession(ctx context.Context, groupID int64, sessionKey string) error {
	return p.Bridge.GatewayDeleteStickySession(ctx, p.GroupID, sessionKey)
}

func (p OpenAISchedulerPorts) RefreshStickySession(ctx context.Context, groupID int64, sessionKey string) error {
	return p.Bridge.GatewayRefreshStickySession(ctx, p.GroupID, sessionKey)
}

func (p OpenAISchedulerPorts) ListSchedulableOpenAIAccounts(ctx context.Context, groupID int64) ([]scheduler.Account, error) {
	accounts, err := p.Bridge.GatewayListSchedulableOpenAIAccounts(ctx, p.GroupID)
	if err != nil {
		return nil, err
	}
	out := make([]scheduler.Account, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		out = append(out, OpenAIAccountToSchedulerAccount(account, p.Bridge.GatewayResolveOpenAITransports(account)))
	}
	return out, nil
}

func (p OpenAISchedulerPorts) GetAccount(ctx context.Context, accountID int64) (scheduler.Account, bool, error) {
	account, err := p.Bridge.GatewayGetOpenAIAccount(ctx, accountID)
	if err != nil || account == nil {
		return scheduler.Account{}, false, err
	}
	return OpenAIAccountToSchedulerAccount(account, p.Bridge.GatewayResolveOpenAITransports(account)), true, nil
}

func (p OpenAISchedulerPorts) AcquireAccountSlot(ctx context.Context, account scheduler.Account) (scheduler.Reservation, error) {
	release, acquired, err := p.Bridge.GatewayAcquireAccountSlot(ctx, account.Snapshot.ID, account.Snapshot.Concurrency)
	if err != nil {
		return scheduler.Reservation{}, err
	}
	return scheduler.Reservation{AccountID: account.Snapshot.ID, Acquired: acquired, Release: release}, nil
}

func (p OpenAISchedulerPorts) WaitPlan(ctx context.Context, account scheduler.Account) domain.AccountWaitPlan {
	timeout, reason := p.Bridge.GatewayDefaultAccountWaitPlan(account.Snapshot.ID, account.Snapshot.Concurrency)
	return domain.AccountWaitPlan{Required: true, Reason: reason, Timeout: timeout}
}

func (p OpenAISchedulerPorts) ReportResult(ctx context.Context, accountID int64, outcome scheduler.ScheduleOutcome) {
	p.Bridge.GatewayReportOpenAIAccountScheduleResult(accountID, outcome.Success, outcome.FirstTokenMs)
}
```

- [ ] **Step 7: Run adapter and service compile tests**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/gateway/adapters ./internal/service -run 'TestOpenAIGatewayService_SelectAccountWithScheduler_PreviousResponseSticky|TestOpenAIGatewayService_SelectAccountWithScheduler_RequiredWSV2_SkipsStickyHTTPAccount'
```

Expected: pass.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/gateway/adapters/openai_scheduler.go backend/internal/gateway/adapters/openai_scheduler_test.go backend/internal/service/openai_account_scheduler.go backend/internal/service/openai_gateway_service.go
git commit -m "feat: bridge legacy OpenAI scheduler dependencies"
```

## Task 7: Handler Planning Helper

**Files:**
- Create: `backend/internal/handler/openai_responses_planning.go`
- Create: `backend/internal/handler/openai_responses_planning_test.go`

- [ ] **Step 1: Write failing helper tests**

Create `backend/internal/handler/openai_responses_planning_test.go`:

```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesPlanningHelperReturnsNormalizedBodyAndLegacyAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/responses/compact", nil)

	account := &service.Account{ID: 9, Name: "openai", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Models: []string{"gpt-5.1"}, Schedulable: true, Concurrency: 1}
	helper := openAIResponsesPlanningHelper{
		selectAccount: func(input openAIResponsesPlanningInput) (*openAIResponsesPlanningResult, error) {
			return &openAIResponsesPlanningResult{
				Plan: domain.RoutingPlan{Request: domain.IngressRequest{RequestID: "req-test"}},
				Account: account,
				NormalizedBody: []byte(`{"model":"gpt-5.1"}`),
			}, nil
		},
	}

	result, err := helper.planAndSelect(c, openAIResponsesPlanningInput{
		body: []byte(`{"model":{"value":"gpt-5.1"}}`),
		subject: openAIPlanningSubject{
			apiKeyID: 1,
			userID:   2,
			groupID:  int64PtrForPlanningTest(3),
		},
		transport: domain.TransportHTTP,
	})
	require.NoError(t, err)
	require.Equal(t, account, result.Account)
	require.JSONEq(t, `{"model":"gpt-5.1"}`, string(result.NormalizedBody))
}

func int64PtrForPlanningTest(v int64) *int64 { return &v }
```

- [ ] **Step 2: Run helper test to verify failure**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/handler -run TestOpenAIResponsesPlanningHelperReturnsNormalizedBodyAndLegacyAccount
```

Expected: fail because helper types do not exist.

- [ ] **Step 3: Implement helper skeleton**

Create `backend/internal/handler/openai_responses_planning.go`:

```go
package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/gateway/adapters"
	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	"github.com/Wei-Shaw/sub2api/internal/gateway/ingress"
	"github.com/Wei-Shaw/sub2api/internal/gateway/planning"
	openai "github.com/Wei-Shaw/sub2api/internal/gateway/provider/openai"
	"github.com/Wei-Shaw/sub2api/internal/gateway/scheduler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type openAIPlanningSubject struct {
	apiKeyID int64
	userID   int64
	groupID  *int64
}

type openAIResponsesPlanningInput struct {
	body                  []byte
	subject               openAIPlanningSubject
	transport             domain.TransportKind
	previousResponseID    string
	sessionKey             string
	requestedModelOverride string
	excludedAccountIDs     map[int64]struct{}
}

type openAIResponsesPlanningResult struct {
	Plan           domain.RoutingPlan
	Account        *service.Account
	NormalizedBody []byte
	ScheduleResult *scheduler.ScheduleResult
}

type openAIResponsesPlanningHelper struct {
	gatewayService     *service.OpenAIGatewayService
	maxAccountSwitches int
	selectAccount      func(input openAIResponsesPlanningInput) (*openAIResponsesPlanningResult, error)
}

func (h openAIResponsesPlanningHelper) planAndSelect(c *gin.Context, input openAIResponsesPlanningInput) (*openAIResponsesPlanningResult, error) {
	if h.selectAccount != nil {
		return h.selectAccount(input)
	}
	if c == nil || c.Request == nil {
		return nil, fmt.Errorf("request context is required")
	}
	groupID := int64(0)
	if input.subject.groupID != nil {
		groupID = *input.subject.groupID
	}
	ingressReq, err := ingress.BuildOpenAIResponses(ingress.OpenAIResponsesInput{
		Request:   c.Request,
		Body:      input.body,
		Transport: input.transport,
		Subject: domain.Subject{
			APIKey: domain.APIKeySnapshot{ID: input.subject.apiKeyID, UserID: input.subject.userID, GroupID: groupID},
			User:   domain.UserSnapshot{ID: input.subject.userID},
			Group:  domain.GroupSnapshot{ID: groupID, Platform: domain.PlatformOpenAI},
		},
	})
	if err != nil {
		return nil, err
	}
	parsed, err := openai.ParseResponses(ingressReq)
	if err != nil {
		return nil, err
	}
	if input.requestedModelOverride != "" {
		parsed.Canonical.RequestedModel = input.requestedModelOverride
	}
	plan := planning.BuildOpenAIResponsesPlan(planning.OpenAIResponsesPlanInput{
		Ingress:            ingressReq,
		Subject:            domain.Subject{APIKey: domain.APIKeySnapshot{ID: input.subject.apiKeyID, UserID: input.subject.userID, GroupID: groupID}, User: domain.UserSnapshot{ID: input.subject.userID}, Group: domain.GroupSnapshot{ID: groupID, Platform: domain.PlatformOpenAI}},
		Parsed:             parsed,
		MaxAccountSwitches: h.maxAccountSwitches,
	})
	if h.gatewayService == nil {
		return nil, fmt.Errorf("gateway service is required")
	}
	ports := adapters.OpenAISchedulerPorts{
		Bridge:         h.gatewayService,
		GroupID:        input.subject.groupID,
		RequestedModel: parsed.Canonical.RequestedModel,
		Excluded:       input.excludedAccountIDs,
	}
	nativeScheduler := scheduler.NewOpenAIScheduler(ports)
	scheduleResult, err := nativeScheduler.Select(contextFromGin(c), scheduler.ScheduleRequest{
		GroupID:            groupID,
		SessionKey:         input.sessionKey,
		PreviousResponseID: input.previousResponseID,
		RequestedModel:     parsed.Canonical.RequestedModel,
		RequiredTransport:  input.transport,
		ExcludedAccountIDs: input.excludedAccountIDs,
	})
	if err != nil {
		return nil, err
	}
	account, _ := scheduleResult.Account.Legacy.(*service.Account)
	if account == nil {
		return nil, fmt.Errorf("selected account legacy handle missing")
	}
	plan.Account.Layer = scheduleResult.Layer
	plan.Account.Account = scheduleResult.Account.Snapshot
	plan.Diagnostics = scheduleResult.Diagnostics
	return &openAIResponsesPlanningResult{
		Plan:           plan,
		Account:        account,
		NormalizedBody: parsed.NormalizedBody,
		ScheduleResult: scheduleResult,
	}, nil
}

func contextFromGin(c *gin.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	return context.Background()
}

func openAIPlanningSubjectFromService(apiKey *service.APIKey, userID int64) openAIPlanningSubject {
	out := openAIPlanningSubject{userID: userID}
	if apiKey != nil {
		out.apiKeyID = apiKey.ID
		out.groupID = apiKey.GroupID
	}
	return out
}

func openAIRequiredTransportForScheduler(transport domain.TransportKind) domain.TransportKind {
	if transport == domain.TransportWebSocket {
		return domain.TransportWebSocket
	}
	return domain.TransportHTTP
}

var _ = http.MethodPost
```

The `var _ = http.MethodPost` line is only needed if `net/http` is imported for compile stabilization during this task. Remove it if the file compiles without the `http` import.

- [ ] **Step 4: Run helper test**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/handler -run TestOpenAIResponsesPlanningHelperReturnsNormalizedBodyAndLegacyAccount
```

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/openai_responses_planning.go backend/internal/handler/openai_responses_planning_test.go
git commit -m "feat: add OpenAI responses planning helper"
```

## Task 8: Live HTTP/SSE Handler Hook

**Files:**
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/handler/openai_responses_planning_test.go`

- [ ] **Step 1: Add failing handler test for scheduler result usage**

Append this test to `backend/internal/handler/openai_responses_planning_test.go`:

```go
func TestOpenAIResponsesPlanningResultConvertsSchedulerSelectionToLegacySelection(t *testing.T) {
	account := &service.Account{ID: 22, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Concurrency: 5}
	result := openAIPlanningResultToLegacySelection(&openAIResponsesPlanningResult{
		Account: account,
		ScheduleResult: &scheduler.ScheduleResult{
			Account: scheduler.Account{Snapshot: domain.AccountSnapshot{ID: 22}},
			Layer:   domain.AccountDecisionLoadBalance,
			Reservation: scheduler.Reservation{
				AccountID: 22,
				Acquired: true,
				Release:  func() {},
			},
		},
	})
	require.NotNil(t, result)
	require.Equal(t, account, result.Account)
	require.True(t, result.Acquired)
	require.NotNil(t, result.ReleaseFunc)
}
```

Add scheduler import to the test file.

- [ ] **Step 2: Run test to verify failure**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/handler -run TestOpenAIResponsesPlanningResultConvertsSchedulerSelectionToLegacySelection
```

Expected: fail because converter does not exist.

- [ ] **Step 3: Add converter helper**

Append to `backend/internal/handler/openai_responses_planning.go`:

```go
func openAIPlanningResultToLegacySelection(result *openAIResponsesPlanningResult) *service.AccountSelectionResult {
	if result == nil || result.Account == nil {
		return nil
	}
	selection := &service.AccountSelectionResult{Account: result.Account}
	if result.ScheduleResult == nil {
		return selection
	}
	selection.Acquired = result.ScheduleResult.Reservation.Acquired
	selection.ReleaseFunc = result.ScheduleResult.Reservation.Release
	if result.ScheduleResult.WaitPlan.Required {
		selection.WaitPlan = &service.AccountWaitPlan{
			AccountID:      result.Account.ID,
			MaxConcurrency: result.Account.Concurrency,
			Timeout:        result.ScheduleResult.WaitPlan.Timeout,
			MaxWaiting:     1,
		}
	}
	return selection
}
```

- [ ] **Step 4: Replace HTTP/SSE selection call**

In `backend/internal/handler/openai_gateway_handler.go`, inside `Responses`, replace this call:

```go
selection, scheduleDecision, err := h.gatewayService.SelectAccountWithScheduler(
	selectCtx,
	apiKey.GroupID,
	previousResponseID,
	sessionHash,
	reqModel,
	failedAccountIDs,
	service.OpenAIUpstreamTransportAny,
)
```

with:

```go
planningHelper := openAIResponsesPlanningHelper{
	gatewayService:     h.gatewayService,
	maxAccountSwitches: h.maxAccountSwitches,
}
planningResult, err := planningHelper.planAndSelect(c, openAIResponsesPlanningInput{
	body:               body,
	subject:            openAIPlanningSubjectFromService(apiKey, subject.UserID),
	transport:          domain.TransportHTTP,
	previousResponseID: previousResponseID,
	sessionKey:          sessionHash,
	excludedAccountIDs:  failedAccountIDs,
})
selection := openAIPlanningResultToLegacySelection(planningResult)
scheduleDecision := openAIPlanningResultToLogDecision(planningResult)
```

Add `github.com/Wei-Shaw/sub2api/internal/gateway/domain` to handler imports.

Add this helper to `openai_responses_planning.go`:

```go
type openAIPlanningLogDecision struct {
	Layer             string
	StickyPreviousHit bool
	StickySessionHit  bool
	CandidateCount    int
	TopK              int
	LatencyMs         int64
	LoadSkew          float64
}

func openAIPlanningResultToLogDecision(result *openAIResponsesPlanningResult) openAIPlanningLogDecision {
	if result == nil || result.ScheduleResult == nil {
		return openAIPlanningLogDecision{}
	}
	layer := string(result.ScheduleResult.Layer)
	return openAIPlanningLogDecision{
		Layer:             layer,
		StickyPreviousHit: result.ScheduleResult.Layer == domain.AccountDecisionPreviousResponseID,
		StickySessionHit:  result.ScheduleResult.Layer == domain.AccountDecisionSessionHash,
		CandidateCount:    result.ScheduleResult.Diagnostics.Total,
		TopK:              result.ScheduleResult.Diagnostics.Eligible,
	}
}
```

- [ ] **Step 5: Ensure normalized compact body is used**

After the new planning call, before forwarding, assign:

```go
if planningResult != nil && len(planningResult.NormalizedBody) > 0 {
	body = planningResult.NormalizedBody
}
```

Keep the existing earlier validation and compact normalization temporarily if removing it would expand the change. If duplicate compact normalization causes tests to fail, remove the old compact normalization block and rely on provider parsing.

- [ ] **Step 6: Run handler compile test**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/handler -run 'TestOpenAIResponsesPlanningResultConvertsSchedulerSelectionToLegacySelection|TestOpenAIResponsesPlanningHelperReturnsNormalizedBodyAndLegacyAccount'
```

Expected: pass.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/handler/openai_gateway_handler.go backend/internal/handler/openai_responses_planning.go backend/internal/handler/openai_responses_planning_test.go
git commit -m "feat: route OpenAI responses selection through native scheduler"
```

## Task 9: Live WebSocket Scheduler Hook

**Files:**
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/handler/openai_responses_planning_test.go`

- [ ] **Step 1: Add WebSocket helper test**

Append to `backend/internal/handler/openai_responses_planning_test.go`:

```go
func TestOpenAIRequiredTransportForSchedulerWebSocket(t *testing.T) {
	require.Equal(t, domain.TransportWebSocket, openAIRequiredTransportForScheduler(domain.TransportWebSocket))
	require.Equal(t, domain.TransportHTTP, openAIRequiredTransportForScheduler(domain.TransportHTTP))
	require.Equal(t, domain.TransportHTTP, openAIRequiredTransportForScheduler(domain.TransportUnknown))
}
```

- [ ] **Step 2: Run test**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/handler -run TestOpenAIRequiredTransportForSchedulerWebSocket
```

Expected: pass if Task 7 helper already exists.

- [ ] **Step 3: Replace WebSocket selection call**

In `ResponsesWebSocket`, replace:

```go
selection, scheduleDecision, err := h.gatewayService.SelectAccountWithScheduler(
	selectCtx,
	apiKey.GroupID,
	previousResponseID,
	sessionHash,
	reqModel,
	nil,
	service.OpenAIUpstreamTransportResponsesWebsocketV2,
)
```

with:

```go
planningHelper := openAIResponsesPlanningHelper{
	gatewayService:     h.gatewayService,
	maxAccountSwitches: h.maxAccountSwitches,
}
planningResult, err := planningHelper.planAndSelect(c, openAIResponsesPlanningInput{
	body:               firstMessage,
	subject:            openAIPlanningSubjectFromService(apiKey, subject.UserID),
	transport:          domain.TransportWebSocket,
	previousResponseID: previousResponseID,
	sessionKey:          sessionHash,
})
selection := openAIPlanningResultToLegacySelection(planningResult)
scheduleDecision := openAIPlanningResultToLogDecision(planningResult)
```

Keep the existing WebSocket accept, first-message validation, billing, token retrieval, relay hooks, and relay call unchanged.

- [ ] **Step 4: Run handler compile tests**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/handler -run 'TestOpenAIRequiredTransportForSchedulerWebSocket|TestOpenAIResponsesPlanning'
```

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/openai_gateway_handler.go backend/internal/handler/openai_responses_planning.go backend/internal/handler/openai_responses_planning_test.go
git commit -m "feat: use native scheduler for OpenAI responses websocket"
```

## Task 10: Route Characterization And Dispatch Tests

**Files:**
- Modify: `backend/internal/server/routes/gateway_test.go`
- Modify: `backend/internal/handler/openai_responses_planning_test.go`

- [ ] **Step 1: Add route characterization tests**

Append to `backend/internal/server/routes/gateway_test.go`:

```go
func TestGatewayRoutesOpenAIResponsesAliasesAreRegistered(t *testing.T) {
	router := gin.New()
	RegisterGatewayRoutes(router, &handler.GatewayHandlers{
		Gateway:       &handler.GatewayHandler{},
		OpenAIGateway: &handler.OpenAIGatewayHandler{},
	}, func(c *gin.Context) {}, nil, nil, nil, nil)

	for _, path := range []string{
		"/responses",
		"/responses/compact",
		"/v1/responses",
		"/v1/responses/compact",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-5.1"}`))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusNotFound, w.Code, "path=%s", path)
	}
}
```

If existing route setup requires concrete middleware services and this test cannot compile as written, adapt it to the existing route test helper in the same file rather than creating a new router setup pattern.

- [ ] **Step 2: Add handler characterization tests for planning result logging**

Append to `backend/internal/handler/openai_responses_planning_test.go`:

```go
func TestOpenAIPlanningResultToLogDecisionMarksStickyLayers(t *testing.T) {
	for _, tt := range []struct {
		layer              domain.AccountDecisionLayer
		wantPreviousSticky bool
		wantSessionSticky  bool
	}{
		{domain.AccountDecisionPreviousResponseID, true, false},
		{domain.AccountDecisionSessionHash, false, true},
		{domain.AccountDecisionLoadBalance, false, false},
	} {
		got := openAIPlanningResultToLogDecision(&openAIResponsesPlanningResult{
			ScheduleResult: &scheduler.ScheduleResult{Layer: tt.layer},
		})
		require.Equal(t, string(tt.layer), got.Layer)
		require.Equal(t, tt.wantPreviousSticky, got.StickyPreviousHit)
		require.Equal(t, tt.wantSessionSticky, got.StickySessionHit)
	}
}
```

- [ ] **Step 3: Run route and handler tests**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/server/routes ./internal/handler -run 'TestGatewayRoutesOpenAIResponses|TestOpenAIPlanningResultToLogDecision'
```

Expected: pass.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/server/routes/gateway_test.go backend/internal/handler/openai_responses_planning_test.go
git commit -m "test: characterize OpenAI responses scheduler dispatch"
```

## Task 11: Boundary And Package Verification

**Files:**
- Modify: `backend/internal/gateway/import_boundary_test.go`
- Test only changes if previous boundary test needs refinement.

- [ ] **Step 1: Strengthen boundary test with explicit allowed legacy packages**

Replace the boundary test body with a helper that supports per-package rules:

```go
func TestGatewayImportBoundaries(t *testing.T) {
	forbiddenImports := []string{
		"github.com/Wei-Shaw/sub2api/ent",
		"github.com/Wei-Shaw/sub2api/internal/config",
		"github.com/Wei-Shaw/sub2api/internal/domain",
		"github.com/Wei-Shaw/sub2api/internal/service",
		"github.com/Wei-Shaw/sub2api/internal/repository",
		"github.com/Wei-Shaw/sub2api/internal/handler",
		"github.com/Wei-Shaw/sub2api/internal/pkg",
		"github.com/Wei-Shaw/sub2api/internal/server",
		"github.com/gin-gonic/gin",
	}

	strictPackages := []string{"domain", "core", "provider", "planning", "scheduler"}
	for _, packageDir := range strictPackages {
		assertPackageAvoidsImports(t, packageDir, forbiddenImports)
	}
}
```

Add helper:

```go
func assertPackageAvoidsImports(t *testing.T, packageDir string, forbiddenImports []string) {
	t.Helper()
	if _, err := os.Stat(packageDir); err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("stat package dir %s: %v", packageDir, err)
	}
	err := filepath.WalkDir(packageDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports for %s: %v", path, err)
		}
		for _, importSpec := range file.Imports {
			importPath := strings.Trim(importSpec.Path.Value, `"`)
			for _, forbiddenImport := range forbiddenImports {
				if importPath == forbiddenImport || strings.HasPrefix(importPath, forbiddenImport+"/") {
					t.Errorf("%s imports forbidden package %q", path, importPath)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk package dir %s: %v", packageDir, err)
	}
}
```

- [ ] **Step 2: Run boundary and gateway package tests**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/gateway/...
```

Expected: pass.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/gateway/import_boundary_test.go
git commit -m "test: enforce expanded gateway import boundaries"
```

## Task 12: Final Verification

**Files:**
- No planned source edits.

- [ ] **Step 1: Run focused gateway tests**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/gateway/...
```

Expected: pass.

- [ ] **Step 2: Run focused handler/routes/service tests**

Run:

```bash
cd backend && go test -count=1 -tags=unit ./internal/handler ./internal/server/routes ./internal/service
```

Expected: pass. If this is too broad because of unrelated existing failures, run the narrower failing package tests surfaced in earlier tasks and document the exact failing package/output before finalizing.

- [ ] **Step 3: Run formatting**

Run:

```bash
cd backend && gofmt -w internal/gateway internal/handler/openai_responses_planning.go internal/handler/openai_responses_planning_test.go internal/handler/openai_gateway_handler.go internal/server/routes/gateway_test.go internal/service/openai_account_scheduler.go internal/service/openai_gateway_service.go
```

Expected: no output.

- [ ] **Step 4: Check final diff**

Run:

```bash
git diff --check
git status --short
```

Expected: `git diff --check` has no output. `git status --short` shows only intended files.

- [ ] **Step 5: Final commit if formatting changed files**

If Step 3 changed files after the previous commits:

```bash
git add backend/internal/gateway backend/internal/handler backend/internal/server/routes/gateway_test.go backend/internal/service
git commit -m "chore: format native OpenAI responses scheduler"
```

If Step 3 made no changes, do not create an empty commit.

## Self-Review

Spec coverage:

- HTTP/SSE ingress, parsing, planning, native scheduler, and live account-selection hook are covered by Tasks 1-8.
- WebSocket planning/scheduler account-selection hook is covered by Task 9.
- Route characterization and OpenAI-vs-non-OpenAI dispatch tests are covered by Task 10.
- Import-boundary enforcement is covered by Tasks 1 and 11.
- Existing forwarding, relay, usage, and billing are intentionally kept in place by Tasks 8 and 9.

No placeholders:

- Every task names concrete files and commands.
- Code-bearing steps include concrete code blocks.
- Open design uncertainties from the spec are converted into explicit implementation decisions: keep function-call-output validation legacy unless it can move without forbidden imports, and keep WebSocket relay legacy.

Type consistency:

- `domain.TransportKind`, `domain.RoutingPlan`, `domain.CandidateDiagnostics`, and `domain.AccountDecisionLayer` are reused consistently.
- `scheduler.ScheduleResult` carries `scheduler.Account` with a legacy handle only in the scheduler/adapters boundary. Legacy `*service.Account` appears only in adapters and handler glue.
