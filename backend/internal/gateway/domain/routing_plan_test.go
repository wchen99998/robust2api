package domain

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRoutingPlanJSONRoundTrip(t *testing.T) {
	now := time.Date(2026, 4, 25, 10, 30, 0, 0, time.UTC)
	resetAt := now.Add(5 * time.Minute)

	plan := RoutingPlan{
		Request: IngressRequest{
			RequestID: "req-123",
			Endpoint:  EndpointOpenAIResponses,
			Platform:  PlatformOpenAI,
			Transport: TransportHTTP,
			Method:    http.MethodPost,
			Path:      "/v1/responses/compact",
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
		},
		Subject: Subject{
			APIKey: APIKeySnapshot{
				ID:      41,
				KeyID:   "key_abc",
				UserID:  7,
				GroupID: 13,
				Policy: GroupPolicy{
					GroupID: 13,
					RateLimit: RateLimitConfig{
						Limit5h: 100,
						Limit1d: 200,
						Limit7d: 700,
					},
				},
			},
			User:  UserSnapshot{ID: 7, Email: "user@example.com", Role: "user"},
			Group: GroupSnapshot{ID: 13, Name: "default"},
		},
		Canonical: CanonicalRequest{
			RequestedModel: "gpt-4.1",
			Headers:        http.Header{"X-Trace": []string{"abc"}},
			Body:           json.RawMessage(`{"model":"gpt-4.1","input":"hello"}`),
			Model: ModelResolution{
				Requested:     "gpt-4.1",
				Canonical:     "gpt-4.1",
				Upstream:      "gpt-4.1-mini",
				Billing:       "gpt-4.1",
				BillingSource: BillingModelSourceChannelMapped,
			},
			Session: SessionInput{
				Key:    "resp_previous",
				Source: SessionSourcePreviousResponseID,
			},
		},
		GroupID: 13,
		Session: SessionDecision{
			Enabled:   true,
			Key:       "resp_previous",
			Source:    SessionSourcePreviousResponseID,
			Sticky:    true,
			AccountID: 99,
		},
		Diagnostics: CandidateDiagnostics{
			Total:       4,
			Eligible:    1,
			Rejected:    3,
			RejectCount: map[RejectionReason]int{RejectionReasonModelUnsupported: 2, RejectionReasonRPMLimited: 1},
			Samples: []CandidateSample{
				{AccountID: 11, Reason: RejectionReasonModelUnsupported, Message: "model not enabled"},
				{AccountID: 12, Reason: RejectionReasonRPMLimited, RetryAfter: 2 * time.Second},
			},
		},
		Account: AccountDecision{
			Layer: AccountDecisionLoadBalance,
			Account: AccountSnapshot{
				ID:       99,
				Name:     "openai-main",
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Capabilities: AccountCapabilities{
					Models:     []string{"gpt-4.1"},
					Transports: []TransportKind{TransportHTTP, TransportSSE},
					Streaming:  true,
				},
			},
			Reservation: AccountReservation{
				AccountID: 99,
				ExpiresAt: resetAt,
			},
			WaitPlan: AccountWaitPlan{
				Required: false,
				Timeout:  10 * time.Second,
			},
		},
		Retry: RetryPlan{
			MaxAttempts:        3,
			RetrySameAccount:   true,
			RetryOtherAccounts: true,
			RetryableStatuses:  []int{429, 500, 502},
			Backoff:            250 * time.Millisecond,
		},
		Billing: BillingLifecyclePlan{
			Mode:          BillingModeStreaming,
			Model:         "gpt-4.1",
			Multiplier:    1.5,
			ReserveTokens: 4096,
			Events:        []BillingEventKind{BillingEventReserve, BillingEventFinalize},
		},
		Debug: DebugPlan{
			Enabled: true,
			BodyFingerprint: BodyFingerprint{
				SHA256: "abcdef",
				Bytes:  128,
			},
		},
		CreatedAt: now,
	}

	assertJSONRoundTrip(t, plan)
}

func TestExecutionReportJSONRoundTrip(t *testing.T) {
	startedAt := time.Date(2026, 4, 25, 10, 30, 0, 0, time.UTC)
	finishedAt := startedAt.Add(3 * time.Second)

	report := ExecutionReport{
		RequestID: "req-123",
		Attempts: []AttemptTrace{
			{
				Attempt:      1,
				AccountID:    99,
				Outcome:      AttemptOutcomeRetryAccount,
				StatusCode:   429,
				ErrorMessage: "rate limited",
				StartedAt:    startedAt,
				FinishedAt:   startedAt.Add(1 * time.Second),
				Duration:     time.Second,
			},
			{
				Attempt:    2,
				AccountID:  100,
				Outcome:    AttemptOutcomeSuccess,
				StatusCode: 200,
				StartedAt:  startedAt.Add(1500 * time.Millisecond),
				FinishedAt: finishedAt,
				Duration:   1500 * time.Millisecond,
			},
		},
		Usage: UsageEvent{
			Kind:             BillingEventFinalize,
			APIKeyID:         41,
			AccountID:        100,
			GroupID:          13,
			Model:            "gpt-4.1",
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
			Cost:             0.42,
		},
		Billing: BillingExecutionReport{
			Mode:        BillingModeStreaming,
			Events:      []BillingEventKind{BillingEventReserve, BillingEventFinalize},
			Reserved:    true,
			Finalized:   true,
			Released:    false,
			ChargedCost: 0.42,
		},
		Error: &GatewayError{
			Code:       "upstream_retry",
			Message:    "first attempt retried",
			StatusCode: 429,
			Retryable:  true,
		},
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}

	assertJSONRoundTrip(t, report)
}

func TestRoutingPlanJSONRedactsSensitiveHeaders(t *testing.T) {
	plan := RoutingPlan{
		Request: IngressRequest{
			RequestID: "req-secret",
			Method:    http.MethodPost,
			Path:      "/v1/responses/compact",
			Header: http.Header{
				"Authorization":     []string{"Bearer sk-live-request-secret"},
				"Cookie":            []string{"session_id=cookie-secret"},
				"X-Api-Key":         []string{"sub2api-key-secret"},
				"OpenAI-API-Key":    []string{"openai-secret"},
				"Anthropic-API-Key": []string{"anthropic-secret"},
				"X-Goog-Api-Key":    []string{"gemini-secret"},
				"Content-Type":      []string{"application/json"},
			},
		},
		Canonical: CanonicalRequest{
			Headers: http.Header{
				"Authorization":     []string{"Bearer sk-live-canonical-secret"},
				"Cookie":            []string{"canonical_cookie=secret"},
				"X-Api-Key":         []string{"canonical-sub2api-secret"},
				"OpenAI-API-Key":    []string{"canonical-openai-secret"},
				"Anthropic-API-Key": []string{"canonical-anthropic-secret"},
				"Gemini-API-Key":    []string{"canonical-gemini-secret"},
				"Accept":            []string{"text/event-stream"},
			},
		},
	}

	payload, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	jsonText := string(payload)

	for _, secret := range []string{
		"sk-live-request-secret",
		"cookie-secret",
		"sub2api-key-secret",
		"openai-secret",
		"anthropic-secret",
		"gemini-secret",
		"sk-live-canonical-secret",
		"canonical_cookie=secret",
		"canonical-sub2api-secret",
		"canonical-openai-secret",
		"canonical-anthropic-secret",
		"canonical-gemini-secret",
	} {
		if strings.Contains(jsonText, secret) {
			t.Fatalf("routing plan JSON leaked secret %q: %s", secret, jsonText)
		}
	}
	for _, expected := range []string{"Content-Type", "application/json", "Accept", "text/event-stream", "[REDACTED]"} {
		if !strings.Contains(jsonText, expected) {
			t.Fatalf("routing plan JSON missing expected value %q: %s", expected, jsonText)
		}
	}
}

func TestRoutingPlanJSONDoesNotSerializeReservationToken(t *testing.T) {
	plan := RoutingPlan{
		Account: AccountDecision{
			Reservation: AccountReservation{
				AccountID: 99,
				Token:     "reservation-token-secret",
				ExpiresAt: time.Date(2026, 4, 25, 10, 35, 0, 0, time.UTC),
			},
		},
	}

	payload, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	jsonText := string(payload)

	if strings.Contains(jsonText, "reservation-token-secret") || strings.Contains(jsonText, `"token"`) {
		t.Fatalf("routing plan JSON leaked reservation token: %s", jsonText)
	}
	if !strings.Contains(jsonText, `"account_id":99`) {
		t.Fatalf("routing plan JSON missing non-sensitive reservation metadata: %s", jsonText)
	}
}

func assertJSONRoundTrip[T any](t *testing.T, value T) {
	t.Helper()

	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got T
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !reflect.DeepEqual(got, value) {
		t.Fatalf("round trip mismatch\n got: %#v\nwant: %#v\njson: %s", got, value, payload)
	}
}
