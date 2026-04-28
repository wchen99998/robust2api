package ingress

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
)

func TestBuildOpenAIResponsesIngressHTTPAliasAndSubpath(t *testing.T) {
	body := []byte(`{"model":"gpt-5.1"}`)
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses/compact/detail", bytes.NewReader(body))
	req.Header.Set("X-Request-Id", "req-123")
	req.Header.Set("Authorization", "Bearer secret")

	subject := domain.Subject{
		APIKey: domain.APIKeySnapshot{ID: 42, KeyID: "key-123", GroupID: 7},
		Group:  domain.GroupSnapshot{ID: 7, Name: "openai", Platform: domain.PlatformOpenAI},
	}

	got, err := BuildOpenAIResponses(OpenAIResponsesInput{
		Request:   req,
		Body:      body,
		Transport: domain.TransportHTTP,
		Subject:   subject,
	})
	if err != nil {
		t.Fatalf("BuildOpenAIResponses returned error: %v", err)
	}

	if got.RequestID != "req-123" {
		t.Fatalf("RequestID = %q, want req-123", got.RequestID)
	}
	if got.Endpoint != domain.EndpointOpenAIResponses {
		t.Fatalf("Endpoint = %q, want %q", got.Endpoint, domain.EndpointOpenAIResponses)
	}
	if got.Platform != domain.PlatformOpenAI {
		t.Fatalf("Platform = %q, want %q", got.Platform, domain.PlatformOpenAI)
	}
	if got.Transport != domain.TransportHTTP {
		t.Fatalf("Transport = %q, want %q", got.Transport, domain.TransportHTTP)
	}
	if got.Method != http.MethodPost {
		t.Fatalf("Method = %q, want %q", got.Method, http.MethodPost)
	}
	if got.Path != "/openai/v1/responses/compact/detail" {
		t.Fatalf("Path = %q, want /openai/v1/responses/compact/detail", got.Path)
	}
	if got.Subpath != "/compact/detail" {
		t.Fatalf("Subpath = %q, want /compact/detail", got.Subpath)
	}
	if !bytes.Equal(got.Body, body) {
		t.Fatalf("Body = %q, want %q", got.Body, body)
	}
	if got.Header.Get("Authorization") != "Bearer secret" {
		t.Fatalf("Authorization header = %q, want Bearer secret", got.Header.Get("Authorization"))
	}
	if got.Header.Get("Authorization") != req.Header.Get("Authorization") {
		t.Fatalf("Header did not retain request Authorization")
	}
	req.Header.Set("Authorization", "Bearer changed")
	if got.Header.Get("Authorization") != "Bearer secret" {
		t.Fatalf("Header was not cloned; got Authorization %q", got.Header.Get("Authorization"))
	}
	body[0] = '['
	if bytes.Equal(got.Body, body) {
		t.Fatalf("Body was not isolated from caller mutation")
	}
}

func TestBuildOpenAIResponsesIngressWebSocket(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)

	got, err := BuildOpenAIResponses(OpenAIResponsesInput{
		Request:   req,
		Transport: domain.TransportWebSocket,
	})
	if err != nil {
		t.Fatalf("BuildOpenAIResponses returned error: %v", err)
	}

	if got.RequestID == "" {
		t.Fatalf("RequestID is empty")
	}
	if got.Endpoint != domain.EndpointOpenAIResponses {
		t.Fatalf("Endpoint = %q, want %q", got.Endpoint, domain.EndpointOpenAIResponses)
	}
	if got.Platform != domain.PlatformOpenAI {
		t.Fatalf("Platform = %q, want %q", got.Platform, domain.PlatformOpenAI)
	}
	if got.Transport != domain.TransportWebSocket {
		t.Fatalf("Transport = %q, want %q", got.Transport, domain.TransportWebSocket)
	}
	if got.Method != http.MethodGet {
		t.Fatalf("Method = %q, want %q", got.Method, http.MethodGet)
	}
	if got.Path != "/v1/responses" {
		t.Fatalf("Path = %q, want /v1/responses", got.Path)
	}
	if got.Subpath != "" {
		t.Fatalf("Subpath = %q, want empty", got.Subpath)
	}
}

func TestBuildOpenAIResponsesNilRequest(t *testing.T) {
	_, err := BuildOpenAIResponses(OpenAIResponsesInput{})
	if err == nil {
		t.Fatalf("BuildOpenAIResponses returned nil error for nil request")
	}
}
