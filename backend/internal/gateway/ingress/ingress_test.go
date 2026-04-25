package ingress

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/stretchr/testify/require"
)

func TestEndpointForPath(t *testing.T) {
	require.Equal(t, core.EndpointResponses, EndpointForPath("/openai/v1/responses/compact"))
	require.Equal(t, core.EndpointMessages, EndpointForPath("/antigravity/v1/messages"))
	require.Equal(t, core.EndpointCountTokens, EndpointForPath("/v1/messages/count_tokens"))
	require.Equal(t, core.EndpointGeminiModels, EndpointForPath("/antigravity/v1beta/models/gemini:generateContent"))
	require.Equal(t, core.EndpointModels, EndpointForPath("/antigravity/models"))
	require.Equal(t, core.EndpointModels, EndpointForPath("/antigravity/v1/models"))
}

func TestIsWebSocket(t *testing.T) {
	require.True(t, IsWebSocket(http.Header{"Upgrade": {"websocket"}}))
	require.False(t, IsWebSocket(http.Header{"Upgrade": {"h2c"}}))
}

func TestWriteResult(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteResult(rec, &core.GatewayResult{
		StatusCode: http.StatusCreated,
		Headers:    http.Header{"X-Test": {"ok"}},
		Body:       []byte(`{"ok":true}`),
	})

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, "ok", rec.Header().Get("X-Test"))
	require.JSONEq(t, `{"ok":true}`, rec.Body.String())
}
