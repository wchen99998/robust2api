package planning

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	openai "github.com/Wei-Shaw/sub2api/internal/gateway/provider/openai"
)

func TestBuildOpenAIResponsesPlanStreamingHTTPFixture(t *testing.T) {
	createdAt := time.Date(2026, 4, 28, 9, 30, 0, 0, time.UTC)
	normalizedBody := []byte(`{"model":"gpt-5.1","stream":true}`)

	plan := BuildOpenAIResponsesPlan(OpenAIResponsesPlanInput{
		Ingress: domain.IngressRequest{
			RequestID: "req-responses-1",
			Endpoint:  domain.EndpointOpenAIResponses,
			Platform:  domain.PlatformOpenAI,
			Transport: domain.TransportHTTP,
			Method:    http.MethodPost,
			Path:      "/v1/responses",
			Header:    http.Header{"Session_id": []string{"sess-header"}},
			Body:      []byte(`{"stream":true,"model":"gpt-5.1"}`),
		},
		Subject: domain.Subject{
			APIKey: domain.APIKeySnapshot{
				ID:      41,
				KeyID:   "key_abc",
				UserID:  7,
				GroupID: 13,
				Policy:  domain.GroupPolicy{GroupID: 13},
			},
			User:  domain.UserSnapshot{ID: 7, Email: "user@example.com", Role: "user"},
			Group: domain.GroupSnapshot{ID: 13, Name: "openai-default", Platform: domain.PlatformOpenAI},
		},
		Parsed: openai.ResponsesParseResult{
			Canonical: domain.CanonicalRequest{
				RequestedModel: "gpt-5.1",
				Headers:        http.Header{"Session_id": []string{"sess-header"}},
				Model: domain.ModelResolution{
					Requested: "gpt-5.1",
					Canonical: "gpt-5.1",
				},
				Session: domain.SessionInput{
					Key:    "sess-header",
					Source: domain.SessionSourceHeader,
				},
			},
			NormalizedBody: normalizedBody,
			Stream:         true,
			BodySHA256:     "abc123",
		},
		MaxAccountSwitches: 3,
		CreatedAt:          createdAt,
	})

	actual, err := json.Marshal(plan)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"request": {
			"request_id": "req-responses-1",
			"endpoint": "openai_responses",
			"platform": "openai",
			"transport": "http",
			"method": "POST",
			"path": "/v1/responses",
			"header": {
				"Session_id": ["sess-header"]
			}
		},
		"subject": {
			"api_key": {
				"id": 41,
				"key_id": "key_abc",
				"user_id": 7,
				"group_id": 13,
				"policy": {
					"group_id": 13,
					"rate_limit": {}
				}
			},
			"user": {
				"id": 7,
				"email": "user@example.com",
				"role": "user"
			},
			"group": {
				"id": 13,
				"name": "openai-default",
				"platform": "openai"
			}
		},
		"canonical": {
			"requested_model": "gpt-5.1",
			"headers": {
				"Session_id": ["sess-header"]
			},
			"model": {
				"requested": "gpt-5.1",
				"canonical": "gpt-5.1"
			},
			"session": {
				"key": "sess-header",
				"source": "header"
			},
			"mutation": {}
		},
		"group_id": 13,
		"session": {
			"enabled": true,
			"key": "sess-header",
			"source": "header",
			"sticky": true
		},
		"diagnostics": {
			"total": 0,
			"eligible": 0,
			"rejected": 0
		},
		"account": {
			"layer": "",
			"account": {
				"id": 0,
				"platform": "",
				"type": "",
				"capabilities": {}
			},
			"reservation": {
				"account_id": 0,
				"expires_at": "0001-01-01T00:00:00Z"
			},
			"wait_plan": {
				"required": false,
				"timeout": 0
			}
		},
		"retry": {
			"max_attempts": 4,
			"retry_same_account": true,
			"retry_other_accounts": true
		},
		"billing": {
			"mode": "streaming",
			"model": "gpt-5.1",
			"events": ["reserve", "finalize", "release"]
		},
		"debug": {
			"enabled": true,
			"body_fingerprint": {
				"sha256": "abc123",
				"bytes": 33
			}
		},
		"created_at": "2026-04-28T09:30:00Z"
	}`, string(actual))
}

func TestBuildOpenAIResponsesPlanNonStreamingDefaults(t *testing.T) {
	plan := BuildOpenAIResponsesPlan(OpenAIResponsesPlanInput{
		Ingress: domain.IngressRequest{Transport: domain.TransportHTTP},
		Subject: domain.Subject{
			APIKey: domain.APIKeySnapshot{GroupID: 21},
		},
		Parsed: openai.ResponsesParseResult{
			Canonical: domain.CanonicalRequest{
				RequestedModel: "gpt-5.1-mini",
				Session:        domain.SessionInput{Source: domain.SessionSourceNone},
			},
			NormalizedBody: []byte(`{"model":"gpt-5.1-mini"}`),
			BodySHA256:     "def456",
		},
		MaxAccountSwitches: -2,
	})

	require.False(t, plan.CreatedAt.IsZero())
	require.Equal(t, 1, plan.Retry.MaxAttempts)
	require.Equal(t, domain.BillingModeToken, plan.Billing.Mode)
	require.Equal(t, "gpt-5.1-mini", plan.Billing.Model)
	require.Equal(t, []domain.BillingEventKind{domain.BillingEventCharge}, plan.Billing.Events)
	require.False(t, plan.Session.Enabled)
	require.Empty(t, plan.Session.Key)
	require.Equal(t, domain.SessionSourceNone, plan.Session.Source)
	require.False(t, plan.Session.Sticky)
}

func TestBuildOpenAIResponsesPlanWebSocketUsesStreamingBilling(t *testing.T) {
	plan := BuildOpenAIResponsesPlan(OpenAIResponsesPlanInput{
		Ingress: domain.IngressRequest{Transport: domain.TransportWebSocket},
		Parsed: openai.ResponsesParseResult{
			Canonical:      domain.CanonicalRequest{RequestedModel: "gpt-5.1"},
			NormalizedBody: []byte(`{"model":"gpt-5.1"}`),
		},
	})

	require.Equal(t, domain.BillingModeStreaming, plan.Billing.Mode)
	require.Equal(t, []domain.BillingEventKind{
		domain.BillingEventReserve,
		domain.BillingEventFinalize,
		domain.BillingEventRelease,
	}, plan.Billing.Events)
	require.False(t, plan.Session.Enabled)
	require.Empty(t, plan.Session.Key)
	require.Equal(t, domain.SessionSourceNone, plan.Session.Source)
	require.False(t, plan.Session.Sticky)
}
