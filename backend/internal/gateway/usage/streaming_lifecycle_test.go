package usage

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestBuildStreamingLifecycleInputUsesRoutingPlanBillingModel(t *testing.T) {
	input := BuildStreamingLifecycleInput(StreamingLifecycleParams{
		RequestID:          "req_123",
		APIKey:             &service.APIKey{ID: 7},
		User:               &service.User{ID: 8},
		Account:            &service.Account{ID: 42},
		RequestPayloadHash: "payload-hash",
		Plan: &core.RoutingPlan{Model: core.ModelResolution{
			RequestedModel: "gpt-5.4",
			BillingModel:   "gpt-5.4-billing",
		}},
	})

	if input.Model != "gpt-5.4-billing" {
		t.Fatalf("model = %q", input.Model)
	}
	if input.RequestID != "req_123" || input.RequestPayloadHash != "payload-hash" {
		t.Fatalf("unexpected input: %+v", input)
	}
}

func TestBuildStreamingLifecycleInputRequestedModelOverride(t *testing.T) {
	input := BuildStreamingLifecycleInput(StreamingLifecycleParams{
		RequestedModel: "gpt-5.4-turn",
		Plan: &core.RoutingPlan{Model: core.ModelResolution{
			BillingModel: "gpt-5.4-billing",
		}},
	})

	if input.Model != "gpt-5.4-turn" {
		t.Fatalf("model = %q", input.Model)
	}
}
