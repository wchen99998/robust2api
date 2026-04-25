package core

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type fakePlanner struct{}

func (fakePlanner) Plan(req IngressRequest) (*RoutingPlan, *CanonicalRequest, error) {
	return &RoutingPlan{
			RequestID: req.RequestID,
			Endpoint:  EndpointResponses,
			Provider:  "openai",
		}, &CanonicalRequest{
			RequestID: req.RequestID,
			Endpoint:  EndpointResponses,
			Provider:  "openai",
		}, nil
}

type fakeRegistry struct {
	adapter ProviderAdapter
}

func (r fakeRegistry) Get(string) (ProviderAdapter, error) {
	return r.adapter, nil
}

type fakeAdapter struct{}

func (fakeAdapter) Provider() string { return "openai" }
func (fakeAdapter) Parse(context.Context, IngressRequest) (*CanonicalRequest, error) {
	return &CanonicalRequest{Endpoint: EndpointResponses, Provider: "openai"}, nil
}
func (fakeAdapter) Prepare(context.Context, RoutingPlan, *service.Account) (*UpstreamRequest, error) {
	return nil, nil
}
func (fakeAdapter) Decode(context.Context, *UpstreamResult) (*GatewayResult, error) {
	return nil, nil
}
func (fakeAdapter) ClassifyError(context.Context, *UpstreamResult) UpstreamErrorDecision {
	return UpstreamErrorDecision{}
}

type retryAdapter struct{}

func (retryAdapter) Provider() string { return service.PlatformOpenAI }
func (retryAdapter) Parse(context.Context, IngressRequest) (*CanonicalRequest, error) {
	return &CanonicalRequest{Endpoint: EndpointResponses, Provider: service.PlatformOpenAI}, nil
}
func (retryAdapter) Prepare(_ context.Context, _ RoutingPlan, account *service.Account) (*UpstreamRequest, error) {
	return &UpstreamRequest{Method: http.MethodPost, URL: "https://example.test/v1/responses", AccountID: account.ID}, nil
}
func (retryAdapter) Decode(_ context.Context, upstream *UpstreamResult) (*GatewayResult, error) {
	return &GatewayResult{StatusCode: upstream.StatusCode, Headers: http.Header{}, Body: upstream.Body}, nil
}
func (retryAdapter) ClassifyError(_ context.Context, upstream *UpstreamResult) UpstreamErrorDecision {
	if upstream == nil || upstream.StatusCode == http.StatusTooManyRequests {
		return UpstreamErrorDecision{Retryable: true, FailoverAccount: true}
	}
	return UpstreamErrorDecision{}
}

type retryPlanner struct{}

func (retryPlanner) Plan(req IngressRequest) (*RoutingPlan, *CanonicalRequest, error) {
	return &RoutingPlan{
			RequestID: req.RequestID,
			Endpoint:  EndpointResponses,
			Provider:  service.PlatformOpenAI,
			Retry:     RetryPlan{MaxAttempts: 2, MaxAccountSwitches: 1},
			Transport: TransportPlan{Method: http.MethodPost},
		}, &CanonicalRequest{
			RequestID: req.RequestID,
			Endpoint:  EndpointResponses,
			Provider:  service.PlatformOpenAI,
		}, nil
}

type fakeAccountSource struct {
	accounts []*service.Account
}

func (s fakeAccountSource) ListAccounts(context.Context, RoutingPlan) ([]*service.Account, error) {
	return s.accounts, nil
}

type fakeSelector struct{}

func (fakeSelector) Select(_ context.Context, _ RoutingPlan, accounts []*service.Account, excluded map[int64]struct{}) AccountSelection {
	for _, account := range accounts {
		if account == nil {
			continue
		}
		if _, ok := excluded[account.ID]; ok {
			continue
		}
		return AccountSelection{
			Account: account,
			Decision: AccountDecision{
				AccountID: account.ID,
				Platform:  account.Platform,
				Type:      account.Type,
			},
			Diagnostics: CandidateDiagnostics{Total: len(accounts), Eligible: 1},
		}
	}
	return AccountSelection{Diagnostics: CandidateDiagnostics{Total: len(accounts)}}
}

type sequenceTransport struct {
	statuses []int
	seen     []int64
}

func (t *sequenceTransport) Do(_ context.Context, req *UpstreamRequest) (*UpstreamResult, error) {
	t.seen = append(t.seen, req.AccountID)
	status := http.StatusOK
	if len(t.statuses) > 0 {
		status = t.statuses[0]
		t.statuses = t.statuses[1:]
	}
	return &UpstreamResult{StatusCode: status, Headers: http.Header{}, Body: []byte(`{}`)}, nil
}

func TestEngineHandlePlansAndParses(t *testing.T) {
	engine := NewEngine(fakePlanner{}, fakeRegistry{adapter: fakeAdapter{}})

	got, err := engine.Handle(context.Background(), IngressRequest{
		RequestID: "req_1",
		Method:    http.MethodPost,
		Path:      "/v1/responses",
	})

	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, got.StatusCode)
	require.Equal(t, "req_1", got.Plan.RequestID)
}

func TestEngineHandleRetriesOnFailoverAccount(t *testing.T) {
	transport := &sequenceTransport{statuses: []int{http.StatusTooManyRequests, http.StatusOK}}
	engine := NewRuntimeEngine(
		retryPlanner{},
		fakeRegistry{adapter: retryAdapter{}},
		fakeAccountSource{accounts: []*service.Account{
			{ID: 10, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
			{ID: 20, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
		}},
		fakeSelector{},
		transport,
		nil,
		nil,
	)

	got, err := engine.Handle(context.Background(), IngressRequest{
		RequestID: "req_retry",
		Method:    http.MethodPost,
		Path:      "/v1/responses",
	})

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, got.StatusCode)
	require.Equal(t, []int64{10, 20}, transport.seen)
	require.Equal(t, int64(20), got.Plan.Account.AccountID)
	require.Equal(t, 2, got.Plan.Account.Attempt)
}

func TestModelsResultFallsBackToDefaultModels(t *testing.T) {
	got := modelsResult(&RoutingPlan{Provider: service.PlatformOpenAI}, nil)

	require.Equal(t, http.StatusOK, got.StatusCode)
	require.Contains(t, string(got.Body), `"object":"list"`)
	require.Contains(t, string(got.Body), `"gpt-5"`)
}

func TestUsageResultQuotaLimited(t *testing.T) {
	got := usageResult(&RoutingPlan{}, IngressRequest{APIKey: &service.APIKey{
		ID:        1,
		Status:    service.StatusAPIKeyActive,
		Quota:     10,
		QuotaUsed: 3,
	}})

	require.Equal(t, http.StatusOK, got.StatusCode)
	require.Contains(t, string(got.Body), `"mode":"quota_limited"`)
	require.Contains(t, string(got.Body), `"remaining":7`)
}

func TestNoAccountsErrorUsesProtocolFamily(t *testing.T) {
	body, _ := noAccountsError(service.PlatformGemini)
	require.JSONEq(t, `{"error":{"code":503,"message":"No available accounts","status":"UNAVAILABLE"}}`, string(body))

	body, _ = noAccountsError(service.PlatformAnthropic)
	require.JSONEq(t, `{"type":"error","error":{"type":"api_error","message":"No available accounts"}}`, string(body))
}
