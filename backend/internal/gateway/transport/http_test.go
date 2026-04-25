package transport

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type upstreamStub struct {
	req *http.Request
}

func (s *upstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.req = req
	return &http.Response{
		StatusCode: http.StatusCreated,
		Header:     http.Header{"X-Upstream": {"ok"}},
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}, nil
}

func (s *upstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestHTTPExecutorDo(t *testing.T) {
	upstream := &upstreamStub{}
	exec := NewHTTPExecutor(upstream, 1024)

	got, err := exec.Do(context.Background(), &core.UpstreamRequest{
		Method:  http.MethodPost,
		URL:     "https://example.com/v1/messages",
		Headers: http.Header{"X-Test": {"ok"}},
		Body:    []byte(`{"hello":"world"}`),
	})

	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, got.StatusCode)
	require.Equal(t, "ok", got.Headers.Get("X-Upstream"))
	require.JSONEq(t, `{"ok":true}`, string(got.Body))
	require.Equal(t, "ok", upstream.req.Header.Get("X-Test"))
}
