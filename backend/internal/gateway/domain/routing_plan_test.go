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
		Plan: RoutingPlan{
			Request: IngressRequest{
				RequestID: "req-123",
				Endpoint:  EndpointOpenAIResponses,
				Platform:  PlatformOpenAI,
				Transport: TransportHTTP,
				Method:    http.MethodPost,
				Path:      "/v1/responses",
			},
			Canonical: CanonicalRequest{
				RequestedModel: "gpt-4.1",
				Model: ModelResolution{
					Requested: "gpt-4.1",
					Canonical: "gpt-4.1",
					Upstream:  "gpt-4.1-mini",
					Billing:   "gpt-4.1",
				},
			},
			GroupID: 13,
			Account: AccountDecision{
				Layer: AccountDecisionLoadBalance,
				Account: AccountSnapshot{
					ID:       100,
					Name:     "openai-failover",
					Platform: PlatformOpenAI,
				},
			},
			Retry: RetryPlan{
				MaxAttempts:        3,
				RetrySameAccount:   true,
				RetryOtherAccounts: true,
			},
			Billing: BillingLifecyclePlan{
				Mode:   BillingModeStreaming,
				Model:  "gpt-4.1",
				Events: []BillingEventKind{BillingEventReserve, BillingEventFinalize},
			},
			CreatedAt: startedAt,
		},
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
		Usage: &UsageEvent{
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

func TestExecutionReportJSONIncludesPlanAndOmitsNilUsage(t *testing.T) {
	startedAt := time.Date(2026, 4, 25, 10, 30, 0, 0, time.UTC)
	report := ExecutionReport{
		RequestID: "req-no-account",
		Plan: RoutingPlan{
			Request: IngressRequest{
				RequestID: "req-no-account",
				Method:    http.MethodPost,
				Path:      "/v1/responses",
			},
			Canonical: CanonicalRequest{
				RequestedModel: "gpt-4.1",
			},
			Diagnostics: CandidateDiagnostics{
				Total:       2,
				Eligible:    0,
				Rejected:    2,
				RejectCount: map[RejectionReason]int{RejectionReasonModelUnsupported: 2},
			},
			Retry: RetryPlan{
				MaxAttempts: 1,
			},
			Billing: BillingLifecyclePlan{
				Mode: BillingModeNone,
			},
			CreatedAt: startedAt,
		},
		Error: &GatewayError{
			Code:       "no_account",
			Message:    "no eligible account",
			StatusCode: http.StatusServiceUnavailable,
			Retryable:  false,
		},
		StartedAt:  startedAt,
		FinishedAt: startedAt,
	}

	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal object: %v", err)
	}
	if _, ok := decoded["plan"]; !ok {
		t.Fatalf("execution report JSON missing plan: %s", payload)
	}
	if _, ok := decoded["usage"]; ok {
		t.Fatalf("execution report JSON should omit nil usage: %s", payload)
	}

	var got ExecutionReport
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if !reflect.DeepEqual(got.Plan, report.Plan) {
		t.Fatalf("plan mismatch\n got: %#v\nwant: %#v\njson: %s", got.Plan, report.Plan, payload)
	}
	if got.Usage != nil {
		t.Fatalf("usage = %#v, want nil", got.Usage)
	}
}

func TestExecutionReportSucceeded(t *testing.T) {
	if got := (ExecutionReport{
		Attempts: []AttemptTrace{{Outcome: AttemptOutcomeSuccess}},
	}).Succeeded(); !got {
		t.Fatalf("Succeeded() = false, want true for successful final attempt")
	}

	if got := (ExecutionReport{
		Attempts: []AttemptTrace{{Outcome: AttemptOutcomeSuccess}},
		Error:    &GatewayError{Code: "failed"},
	}).Succeeded(); got {
		t.Fatalf("Succeeded() = true, want false when report has error")
	}

	if got := (ExecutionReport{
		Attempts: []AttemptTrace{{Outcome: AttemptOutcomeRetryAccount}},
	}).Succeeded(); got {
		t.Fatalf("Succeeded() = true, want false for non-success final attempt")
	}

	if got := (ExecutionReport{}).Succeeded(); got {
		t.Fatalf("Succeeded() = true, want false without attempts")
	}
}

func TestRoutingPlanJSONRedactsSensitiveHeaders(t *testing.T) {
	sensitiveHeaders := []string{
		"Authorization",
		"Cookie",
		"X-Api-Key",
		"OpenAI-API-Key",
		"Anthropic-API-Key",
		"X-Goog-Api-Key",
		"OpenAIApiKey",
		"Access-Token",
		"Refresh-Token",
		"ID-Token",
		"X-Auth-Token",
		"X-Client-Secret",
		"Private-Token",
		"Session-Token",
		"X-Service-Credential",
		"X-User-Password",
	}
	requestHeaders := make(http.Header, len(sensitiveHeaders)+1)
	canonicalHeaders := make(http.Header, len(sensitiveHeaders)+1)
	var secrets []string
	for _, header := range sensitiveHeaders {
		requestSecret := "request-secret-" + strings.ToLower(header)
		canonicalSecret := "canonical-secret-" + strings.ToLower(header)
		requestHeaders.Set(header, requestSecret)
		canonicalHeaders.Set(header, canonicalSecret)
		secrets = append(secrets, requestSecret, canonicalSecret)
	}
	requestHeaders.Set("Content-Type", "application/json")
	canonicalHeaders.Set("Accept", "text/event-stream")

	plan := RoutingPlan{
		Request: IngressRequest{
			RequestID: "req-secret",
			Method:    http.MethodPost,
			Path:      "/v1/responses/compact",
			Header:    requestHeaders,
		},
		Canonical: CanonicalRequest{
			Headers: canonicalHeaders,
		},
	}

	payload, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	jsonText := string(payload)

	for _, secret := range secrets {
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

func TestIsSensitiveHeader(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		sensitive bool
	}{
		{name: "authorization", header: "Authorization", sensitive: true},
		{name: "cookie", header: "Cookie", sensitive: true},
		{name: "x api key", header: "X-Api-Key", sensitive: true},
		{name: "provider api key", header: "OpenAI-API-Key", sensitive: true},
		{name: "unseparated api key", header: "OpenAIApiKey", sensitive: true},
		{name: "access token", header: "Access-Token", sensitive: true},
		{name: "refresh token", header: "Refresh-Token", sensitive: true},
		{name: "id token", header: "ID-Token", sensitive: true},
		{name: "auth token", header: "X-Auth-Token", sensitive: true},
		{name: "client secret", header: "X-Client-Secret", sensitive: true},
		{name: "private token", header: "Private-Token", sensitive: true},
		{name: "session token", header: "Session-Token", sensitive: true},
		{name: "credential substring", header: "X-Service-Credential", sensitive: true},
		{name: "password substring", header: "X-User-Password", sensitive: true},
		{name: "content type", header: "Content-Type", sensitive: false},
		{name: "accept", header: "Accept", sensitive: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSensitiveHeader(tt.header); got != tt.sensitive {
				t.Fatalf("isSensitiveHeader(%q) = %t, want %t", tt.header, got, tt.sensitive)
			}
		})
	}
}

func TestRoutingPlanJSONDoesNotSerializeCanonicalBody(t *testing.T) {
	plan := RoutingPlan{
		Canonical: CanonicalRequest{
			Body: json.RawMessage(`{"input":"canonical-body-only-secret-9b8f6a5c"}`),
		},
	}

	payload, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	jsonText := string(payload)

	if strings.Contains(jsonText, "canonical-body-only-secret-9b8f6a5c") {
		t.Fatalf("routing plan JSON leaked canonical body secret: %s", jsonText)
	}

	var decoded struct {
		Canonical map[string]json.RawMessage `json:"canonical"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := decoded.Canonical["body"]; ok {
		t.Fatalf("routing plan JSON serialized canonical body field: %s", jsonText)
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
