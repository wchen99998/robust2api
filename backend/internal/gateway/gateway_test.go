package gateway

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestNewCoreHandlesPlanningAndParsing(t *testing.T) {
	gatewayCore := NewCore(1)

	got, err := gatewayCore.Handle(context.Background(), core.IngressRequest{
		RequestID: "req_1",
		Method:    http.MethodPost,
		Path:      "/openai/v1/responses",
		RawPath:   "/openai/v1/responses",
		Headers:   http.Header{"Authorization": {"Bearer secret"}},
		Body:      []byte(`{"model":"gpt-5"}`),
	})

	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, got.StatusCode)
	require.Equal(t, core.EndpointResponses, got.Plan.Endpoint)
	require.Equal(t, "openai", got.Plan.Provider)
	require.Equal(t, []string{"[redacted]"}, got.Plan.Debug.SafeHeaders["Authorization"])
}

func TestNewRuntimeCoreExecutesHTTPPath(t *testing.T) {
	repo := runtimeAccountRepo{accounts: []service.Account{{
		ID:          1,
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com",
		},
	}}}
	upstream := &runtimeUpstream{}

	gatewayCore := NewRuntimeCore(repo, upstream, 1, 1024)
	got, err := gatewayCore.Handle(context.Background(), core.IngressRequest{
		RequestID: "req_1",
		Method:    http.MethodPost,
		Path:      "/openai/v1/responses",
		RawPath:   "/openai/v1/responses",
		Body:      []byte(`{"model":"gpt-5"}`),
	})

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, got.StatusCode)
	require.JSONEq(t, `{"id":"resp_1"}`, string(got.Body))
	require.Equal(t, "https://api.openai.com/v1/responses", upstream.req.URL.String())
	require.Equal(t, "Bearer sk-test", upstream.req.Header.Get("authorization"))
}

type runtimeAccountRepo struct {
	service.AccountRepository
	accounts []service.Account
}

func (r runtimeAccountRepo) ListSchedulableByPlatform(context.Context, string) ([]service.Account, error) {
	return r.accounts, nil
}

func (r runtimeAccountRepo) ListSchedulable(context.Context) ([]service.Account, error) {
	return r.accounts, nil
}

type runtimeUpstream struct {
	req *http.Request
}

func (u *runtimeUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.req = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"content-type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp_1"}`)),
	}, nil
}

func (u *runtimeUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}
