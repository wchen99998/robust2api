package usage

import (
	"context"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
)

type Extractor struct{}

func NewExtractor() *Extractor {
	return &Extractor{}
}

func (e *Extractor) Extract(_ context.Context, plan core.RoutingPlan, result *core.GatewayResult) core.UsageEvent {
	if result == nil {
		return FromPlan(&plan)
	}
	switch plan.Endpoint {
	case core.EndpointResponses, core.EndpointChatCompletions:
		return FromOpenAIResponses(&plan, result.Body)
	case core.EndpointMessages, core.EndpointCountTokens:
		return FromAnthropicMessages(&plan, result.Body)
	default:
		return FromPlan(&plan)
	}
}

func FromPlan(plan *core.RoutingPlan) core.UsageEvent {
	if plan == nil {
		return core.UsageEvent{}
	}
	return core.UsageEvent{
		RequestID:          plan.RequestID,
		Model:              plan.Model.BillingModel,
		UpstreamModel:      plan.Model.UpstreamModel,
		Stream:             plan.Billing.Streaming,
		RequestPayloadHash: plan.Billing.RequestPayloadHash,
		AccountID:          plan.Account.AccountID,
	}
}

type openAIUsagePayload struct {
	Usage struct {
		InputTokens        int `json:"input_tokens"`
		OutputTokens       int `json:"output_tokens"`
		TotalTokens        int `json:"total_tokens"`
		InputTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
		OutputTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
	} `json:"usage"`
}

func FromOpenAIResponses(plan *core.RoutingPlan, body []byte) core.UsageEvent {
	event := FromPlan(plan)
	var payload openAIUsagePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return event
	}
	event.InputTokens = payload.Usage.InputTokens
	event.OutputTokens = payload.Usage.OutputTokens
	event.CacheReadTokens = payload.Usage.InputTokensDetails.CachedTokens
	return event
}

type anthropicUsagePayload struct {
	Usage struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

func FromAnthropicMessages(plan *core.RoutingPlan, body []byte) core.UsageEvent {
	event := FromPlan(plan)
	var payload anthropicUsagePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return event
	}
	event.InputTokens = payload.Usage.InputTokens
	event.OutputTokens = payload.Usage.OutputTokens
	event.CacheCreateTokens = payload.Usage.CacheCreationInputTokens
	event.CacheReadTokens = payload.Usage.CacheReadInputTokens
	return event
}
