package transport

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHTTPTransportRoundTrip(t *testing.T) {
	tr := HTTPTransport{Client: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://example.com/v1/responses" {
			t.Fatalf("url = %q", req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		}, nil
	})}
	result, err := tr.RoundTrip(context.Background(), &core.UpstreamRequest{
		Method: http.MethodPost,
		URL:    "https://example.com/v1/responses",
	})
	if err != nil {
		t.Fatalf("RoundTrip returned error: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", result.StatusCode)
	}
	if string(result.Body) != `{"ok":true}` {
		t.Fatalf("body = %q", string(result.Body))
	}
}

func TestHTTPTransportBodyLimit(t *testing.T) {
	tr := HTTPTransport{
		MaxBodyBytes: 4,
		Client: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("12345")),
			}, nil
		}),
	}
	_, err := tr.RoundTrip(context.Background(), &core.UpstreamRequest{Method: http.MethodGet, URL: "https://example.com"})
	if err == nil {
		t.Fatalf("expected body limit error")
	}
}
