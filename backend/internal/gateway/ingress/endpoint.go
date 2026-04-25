package ingress

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
)

func NormalizeEndpoint(method, rawPath string) core.EndpointKind {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return core.EndpointUnknown
	}
	if strings.Contains(path, "/v1beta/") || strings.Contains(path, "/v1/") && strings.Contains(path, ":generateContent") {
		return core.EndpointGeminiNative
	}
	if strings.Contains(path, "/responses") {
		if strings.EqualFold(method, http.MethodGet) {
			return core.EndpointResponsesWebSocket
		}
		return core.EndpointResponses
	}
	if strings.HasSuffix(path, "/messages/count_tokens") {
		return core.EndpointMessagesCountTokens
	}
	if strings.HasSuffix(path, "/chat/completions") {
		return core.EndpointChatCompletions
	}
	if strings.HasSuffix(path, "/messages") {
		return core.EndpointMessages
	}
	return core.EndpointUnknown
}

func ResponsesSubpath(rawPath string) string {
	path := strings.TrimRight(strings.TrimSpace(rawPath), "/")
	idx := strings.LastIndex(path, "/responses")
	if idx < 0 {
		return ""
	}
	suffix := path[idx+len("/responses"):]
	if suffix == "/" {
		return ""
	}
	return suffix
}
