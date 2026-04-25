package ingress

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
)

func EndpointForPath(path string) core.EndpointKind {
	switch {
	case strings.Contains(path, "/messages/count_tokens"):
		return core.EndpointCountTokens
	case strings.Contains(path, "/messages"):
		return core.EndpointMessages
	case strings.Contains(path, "/chat/completions"):
		return core.EndpointChatCompletions
	case strings.Contains(path, "/responses"):
		return core.EndpointResponses
	case strings.Contains(path, "/v1beta/models"):
		return core.EndpointGeminiModels
	case strings.HasSuffix(path, "/models"):
		return core.EndpointModels
	case strings.HasSuffix(path, "/usage"):
		return core.EndpointUsage
	case strings.HasPrefix(path, "/antigravity"):
		return core.EndpointAntigravity
	default:
		return core.EndpointUnknown
	}
}

func IsWebSocket(headers http.Header) bool {
	return strings.EqualFold(headers.Get("Upgrade"), "websocket")
}

func WriteResult(w http.ResponseWriter, result *core.GatewayResult) {
	if result == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	for key, values := range result.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	status := result.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	if len(result.Body) > 0 {
		_, _ = w.Write(result.Body)
	}
}
