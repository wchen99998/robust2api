package core

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type fakeAdapter struct{}

func (fakeAdapter) Provider() string { return service.PlatformOpenAI }

func (fakeAdapter) Parse(_ context.Context, req IngressRequest) (*CanonicalRequest, error) {
	return &CanonicalRequest{
		RequestID:      req.RequestID,
		Endpoint:       EndpointResponses,
		Provider:       service.PlatformOpenAI,
		RequestedModel: "gpt-5.4",
		Body:           req.Body,
		Headers:        req.Headers.Clone(),
	}, nil
}

func (fakeAdapter) Prepare(_ context.Context, plan RoutingPlan, account *service.Account) (*UpstreamRequest, error) {
	return &UpstreamRequest{
		Method: http.MethodPost,
		URL:    "https://example.com/v1/responses",
	}, nil
}

func (fakeAdapter) Decode(_ context.Context, upstream *UpstreamResult) (*GatewayResult, error) {
	return &GatewayResult{Status: upstream.StatusCode, Headers: upstream.Headers.Clone(), Body: append([]byte(nil), upstream.Body...)}, nil
}

func (fakeAdapter) ClassifyError(_ context.Context, upstream *UpstreamResult) UpstreamErrorDecision {
	if upstream == nil {
		return UpstreamErrorDecision{ClientStatus: http.StatusBadGateway, ClientErrorType: "upstream_error", ClientErrorMessage: "Request failed"}
	}
	return UpstreamErrorDecision{ClientStatus: upstream.StatusCode, ClientErrorType: "upstream_error", ClientErrorMessage: "HTTP error"}
}

type fakePlanner struct{}

func (fakePlanner) Build(_ context.Context, req *CanonicalRequest, _ *service.APIKey) (*RoutingPlan, error) {
	return &RoutingPlan{
		RequestID: req.RequestID,
		Endpoint:  req.Endpoint,
		Provider:  req.Provider,
		Model:     ModelResolution{RequestedModel: req.RequestedModel, UpstreamModel: "gpt-5.4-upstream", BillingModel: "gpt-5.4-upstream"},
	}, nil
}

type fakeSelector struct {
	selection *AccountSelection
	err       error
}

func (s fakeSelector) Select(_ context.Context, plan RoutingPlan) (*AccountSelection, error) {
	if s.err != nil {
		return s.selection, s.err
	}
	if s.selection != nil {
		return s.selection, nil
	}
	plan.Account = AccountDecision{AccountID: 1, AccountName: "primary", Platform: service.PlatformOpenAI}
	return &AccountSelection{Plan: plan, Account: &service.Account{ID: 1, Name: "primary", Platform: service.PlatformOpenAI}}, nil
}

type fakeTransport struct {
	result   *UpstreamResult
	err      error
	lastBody []byte
}

func (t *fakeTransport) RoundTrip(_ context.Context, req *UpstreamRequest) (*UpstreamResult, error) {
	if t.err != nil {
		return nil, t.err
	}
	t.lastBody = append([]byte(nil), req.Body...)
	return t.result, nil
}

type fakeUsageMapper struct{}

func (fakeUsageMapper) Map(_ context.Context, plan RoutingPlan, account *service.Account, _ *GatewayResult) (*UsageEvent, error) {
	return &UsageEvent{RequestID: plan.RequestID, AccountID: account.ID, BillingModel: plan.Billing.Model}, nil
}

func TestPipelineHandleSuccess(t *testing.T) {
	transport := &fakeTransport{result: &UpstreamResult{
		StatusCode: http.StatusOK,
		Headers:    http.Header{"Content-Type": []string{"application/json"}},
		Body:       []byte(`{"ok":true}`),
	}}
	pipeline := Pipeline{
		Adapter:   fakeAdapter{},
		Planner:   fakePlanner{},
		Selector:  fakeSelector{},
		Transport: transport,
		Usage:     fakeUsageMapper{},
	}

	result, err := pipeline.Handle(context.Background(), IngressRequest{
		RequestID: "req_123",
		Body:      []byte(`{"model":"gpt-5.4"}`),
		Headers:   http.Header{},
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if result.Status != http.StatusOK {
		t.Fatalf("status = %d", result.Status)
	}
	if result.Plan == nil || result.Plan.Account.AccountID != 1 {
		t.Fatalf("missing plan/account: %+v", result.Plan)
	}
	if result.Usage == nil || result.Usage.AccountID != 1 {
		t.Fatalf("missing usage: %+v", result.Usage)
	}
	if string(transport.lastBody) != `{"model":"gpt-5.4"}` {
		t.Fatalf("transport body = %s", transport.lastBody)
	}
}

func TestPipelineHandleSelectionErrorReturnsPlanDiagnostics(t *testing.T) {
	plan := RoutingPlan{
		Candidates: CandidateDiagnostics{
			Total:    1,
			Rejected: map[RejectionReason]int{RejectedUnschedulable: 1},
		},
	}
	pipeline := Pipeline{
		Adapter:   fakeAdapter{},
		Planner:   fakePlanner{},
		Selector:  fakeSelector{selection: &AccountSelection{Plan: plan}, err: errors.New("no account")},
		Transport: &fakeTransport{},
	}

	result, err := pipeline.Handle(context.Background(), IngressRequest{RequestID: "req_123", Headers: http.Header{}})
	if err == nil {
		t.Fatalf("expected selection error")
	}
	if result == nil || result.Status != http.StatusServiceUnavailable {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Plan.Candidates.Rejected[RejectedUnschedulable] != 1 {
		t.Fatalf("missing rejection diagnostics: %+v", result.Plan.Candidates)
	}
}
