package ingress

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestNewHTTPRequestIngressNormalizesOpenAIResponses(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/openai/v1/responses/compact", strings.NewReader(`{"model":"gpt-5.4"}`))
	r.Header.Set("X-Request-ID", "req_123")
	r.Header.Set("User-Agent", "client")

	req := NewHTTPRequestIngress(r, []byte(`{"model":"gpt-5.4"}`), "127.0.0.1", &service.APIKey{ID: 7}, &service.User{ID: 8})

	if req.RequestID != "req_123" {
		t.Fatalf("request id = %q", req.RequestID)
	}
	if req.Endpoint != core.EndpointResponses {
		t.Fatalf("endpoint = %q", req.Endpoint)
	}
	if req.ClientIP != "127.0.0.1" {
		t.Fatalf("client ip = %q", req.ClientIP)
	}
}

func TestParseOpenAIResponsesBuildsCanonicalRequest(t *testing.T) {
	headers := http.Header{}
	headers.Set("User-Agent", "client")
	headers.Set("X-OpenAI-Session", "header-session")
	headers.Set("OpenAI-Other-Test", "preserved")
	req := core.IngressRequest{
		RequestID: "req_123",
		Method:    http.MethodPost,
		Path:      "/v1/responses",
		Headers:   headers,
		Body:      []byte(`{"model":"gpt-5.4","stream":true}`),
		ClientIP:  "127.0.0.1",
		APIKey:    &service.APIKey{ID: 7},
	}

	canonical, err := ParseOpenAIResponses(req, core.SessionInput{})
	if err != nil {
		t.Fatalf("ParseOpenAIResponses returned error: %v", err)
	}
	if canonical.Endpoint != core.EndpointResponses {
		t.Fatalf("endpoint = %q", canonical.Endpoint)
	}
	if canonical.RequestedModel != "gpt-5.4" || !canonical.Stream {
		t.Fatalf("unexpected model/stream: %q %v", canonical.RequestedModel, canonical.Stream)
	}
	if canonical.Session.Key != "header-session" || canonical.Session.APIKeyID != 7 {
		t.Fatalf("unexpected session: %+v", canonical.Session)
	}
	if canonical.Session.ClientIP != "127.0.0.1" || canonical.Session.UserAgent != "client" {
		t.Fatalf("unexpected session metadata: %+v", canonical.Session)
	}
}

func TestParseOpenAIResponsesAllowsExplicitSessionOverride(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-OpenAI-Session", "header-session")
	canonical, err := ParseOpenAIResponses(core.IngressRequest{
		Method:  http.MethodPost,
		Path:    "/v1/responses",
		Headers: headers,
		Body:    []byte(`{"model":"gpt-5.4"}`),
	}, core.SessionInput{Key: "computed-session"})
	if err != nil {
		t.Fatalf("ParseOpenAIResponses returned error: %v", err)
	}
	if canonical.Session.Key != "computed-session" {
		t.Fatalf("session key = %q", canonical.Session.Key)
	}
}
