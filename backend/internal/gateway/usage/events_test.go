package usage

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/stretchr/testify/require"
)

func TestFromOpenAIResponses(t *testing.T) {
	plan := &core.RoutingPlan{
		RequestID: "req_1",
		Account:   core.AccountDecision{AccountID: 5},
		Model:     core.ModelResolution{BillingModel: "gpt-5", UpstreamModel: "gpt-5"},
		Billing:   core.BillingPlan{RequestPayloadHash: "hash"},
	}

	got := FromOpenAIResponses(plan, []byte(`{
		"usage": {
			"input_tokens": 10,
			"output_tokens": 20,
			"input_tokens_details": {"cached_tokens": 3}
		}
	}`))

	require.Equal(t, "req_1", got.RequestID)
	require.Equal(t, int64(5), got.AccountID)
	require.Equal(t, "gpt-5", got.Model)
	require.Equal(t, 10, got.InputTokens)
	require.Equal(t, 20, got.OutputTokens)
	require.Equal(t, 3, got.CacheReadTokens)
	require.Equal(t, "hash", got.RequestPayloadHash)
}

func TestExtractorUsesEndpointProtocolUsage(t *testing.T) {
	plan := core.RoutingPlan{
		RequestID: "req_2",
		Endpoint:  core.EndpointMessages,
		Account:   core.AccountDecision{AccountID: 6},
		Model:     core.ModelResolution{BillingModel: "claude-sonnet-4-5", UpstreamModel: "claude-sonnet-4-5"},
		Billing:   core.BillingPlan{RequestPayloadHash: "hash2"},
	}

	got := NewExtractor().Extract(context.Background(), plan, &core.GatewayResult{Body: []byte(`{
		"usage": {
			"input_tokens": 11,
			"output_tokens": 22,
			"cache_creation_input_tokens": 4,
			"cache_read_input_tokens": 5
		}
	}`)})

	require.Equal(t, "req_2", got.RequestID)
	require.Equal(t, int64(6), got.AccountID)
	require.Equal(t, "claude-sonnet-4-5", got.Model)
	require.Equal(t, 11, got.InputTokens)
	require.Equal(t, 22, got.OutputTokens)
	require.Equal(t, 4, got.CacheCreateTokens)
	require.Equal(t, 5, got.CacheReadTokens)
	require.Equal(t, "hash2", got.RequestPayloadHash)
}
