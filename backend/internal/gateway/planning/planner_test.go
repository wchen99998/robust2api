package planning

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestDetectEndpoint(t *testing.T) {
	tests := []struct {
		path string
		want core.EndpointKind
	}{
		{"/v1/responses", core.EndpointResponses},
		{"/openai/v1/responses/compact", core.EndpointResponses},
		{"/v1/messages", core.EndpointMessages},
		{"/v1/messages/count_tokens", core.EndpointCountTokens},
		{"/v1/chat/completions", core.EndpointChatCompletions},
		{"/v1beta/models/gemini:generateContent", core.EndpointGeminiModels},
		{"/antigravity/v1/messages", core.EndpointMessages},
		{"/antigravity/models", core.EndpointModels},
		{"/antigravity/v1/models", core.EndpointModels},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, DetectEndpoint(tt.path), tt.path)
	}
}

func TestPlannerProducesSerializableRoutingPlan(t *testing.T) {
	groupID := int64(10)
	req := core.IngressRequest{
		RequestID: "req_1",
		Method:    http.MethodPost,
		Path:      "/openai/v1/responses/compact",
		RawPath:   "/openai/v1/responses/compact",
		Headers: http.Header{
			"Authorization": {"Bearer secret"},
			"User-Agent":    {"codex"},
		},
		Body:     []byte(`{"model":"gpt-5","stream":true}`),
		ClientIP: "127.0.0.1",
		APIKey: &service.APIKey{
			ID:      7,
			GroupID: &groupID,
			Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI},
		},
	}

	plan, canonical, err := NewPlanner(2).Plan(req)

	require.NoError(t, err)
	require.Equal(t, core.EndpointResponses, canonical.Endpoint)
	require.Equal(t, "/compact", canonical.Subpath)
	require.Equal(t, service.PlatformOpenAI, plan.Provider)
	require.Equal(t, "gpt-5", plan.Model.RequestedModel)
	require.Equal(t, "gpt-5", plan.Model.UpstreamModel)
	require.True(t, plan.Transport.Stream)
	require.Equal(t, []string{"[redacted]"}, plan.Debug.SafeHeaders["Authorization"])
	require.Equal(t, 3, plan.Retry.MaxAttempts)
}
