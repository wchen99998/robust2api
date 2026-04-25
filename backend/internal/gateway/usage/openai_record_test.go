package usage

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestBuildOpenAIRecordUsageInputUsesRoutingPlanChannelFields(t *testing.T) {
	plan := &core.RoutingPlan{
		Model: core.ModelResolution{
			RequestedModel:     "gpt-5.4",
			ChannelID:          11,
			ChannelMappedModel: "gpt-5.4-channel",
			UpstreamModel:      "gpt-5.4-channel",
			BillingModelSource: service.BillingModelSourceChannelMapped,
		},
	}
	input := BuildOpenAIRecordUsageInput(OpenAIRecordUsageParams{
		Result: &service.OpenAIForwardResult{
			RequestID:     "req_upstream",
			UpstreamModel: "gpt-5.4-account",
		},
		APIKey:             &service.APIKey{ID: 7},
		Account:            &service.Account{ID: 42},
		RequestPayloadHash: "payload-hash",
		Plan:               plan,
	})

	if input.ChannelID != 11 {
		t.Fatalf("channel id = %d", input.ChannelID)
	}
	if input.OriginalModel != "gpt-5.4" {
		t.Fatalf("original model = %q", input.OriginalModel)
	}
	if input.ChannelMappedModel != "gpt-5.4-channel" {
		t.Fatalf("channel mapped model = %q", input.ChannelMappedModel)
	}
	if input.BillingModelSource != service.BillingModelSourceChannelMapped {
		t.Fatalf("billing model source = %q", input.BillingModelSource)
	}
	if input.ModelMappingChain != "gpt-5.4→gpt-5.4-channel→gpt-5.4-account" {
		t.Fatalf("mapping chain = %q", input.ModelMappingChain)
	}
}

func TestBuildOpenAIRecordUsageInputHandlesUnmappedUpstreamModel(t *testing.T) {
	input := BuildOpenAIRecordUsageInput(OpenAIRecordUsageParams{
		Result: &service.OpenAIForwardResult{UpstreamModel: "gpt-5.4-upstream"},
		Plan: &core.RoutingPlan{Model: core.ModelResolution{
			RequestedModel:     "gpt-5.4",
			ChannelMappedModel: "gpt-5.4",
			UpstreamModel:      "gpt-5.4",
		}},
	})

	if input.ModelMappingChain != "gpt-5.4→gpt-5.4-upstream" {
		t.Fatalf("mapping chain = %q", input.ModelMappingChain)
	}
}

func TestBuildOpenAIRecordUsageInputSupportsRequestedModelOverride(t *testing.T) {
	input := BuildOpenAIRecordUsageInput(OpenAIRecordUsageParams{
		Result:         &service.OpenAIForwardResult{UpstreamModel: "gpt-5.4-channel"},
		RequestedModel: "gpt-5.4-turn",
		Plan: &core.RoutingPlan{Model: core.ModelResolution{
			RequestedModel:     "gpt-5.4-first",
			ChannelMappedModel: "gpt-5.4-channel",
		}},
	})

	if input.OriginalModel != "gpt-5.4-turn" {
		t.Fatalf("original model = %q", input.OriginalModel)
	}
	if input.ModelMappingChain != "gpt-5.4-turn→gpt-5.4-channel" {
		t.Fatalf("mapping chain = %q", input.ModelMappingChain)
	}
}
