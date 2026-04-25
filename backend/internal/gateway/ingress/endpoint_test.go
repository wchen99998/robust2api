package ingress

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
)

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   core.EndpointKind
	}{
		{name: "responses post", method: http.MethodPost, path: "/openai/v1/responses", want: core.EndpointResponses},
		{name: "responses nested", method: http.MethodPost, path: "/v1/responses/compact", want: core.EndpointResponses},
		{name: "responses websocket", method: http.MethodGet, path: "/openai/v1/responses", want: core.EndpointResponsesWebSocket},
		{name: "count tokens", method: http.MethodPost, path: "/v1/messages/count_tokens", want: core.EndpointMessagesCountTokens},
		{name: "chat completions", method: http.MethodPost, path: "/v1/chat/completions", want: core.EndpointChatCompletions},
		{name: "messages", method: http.MethodPost, path: "/v1/messages", want: core.EndpointMessages},
		{name: "unknown", method: http.MethodPost, path: "/v1/embeddings", want: core.EndpointUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeEndpoint(tt.method, tt.path); got != tt.want {
				t.Fatalf("NormalizeEndpoint(%q, %q) = %q, want %q", tt.method, tt.path, got, tt.want)
			}
		})
	}
}

func TestResponsesSubpath(t *testing.T) {
	tests := map[string]string{
		"/v1/responses":                       "",
		"/v1/responses/":                      "",
		"/openai/v1/responses/compact":        "/compact",
		"/openai/v1/responses/compact/detail": "/compact/detail",
		"/v1/messages":                        "",
	}
	for path, want := range tests {
		if got := ResponsesSubpath(path); got != want {
			t.Fatalf("ResponsesSubpath(%q) = %q, want %q", path, got, want)
		}
	}
}
