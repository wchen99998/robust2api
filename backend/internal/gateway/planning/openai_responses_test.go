package planning

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type channelResolverStub struct {
	result service.ChannelMappingResult
}

func (s channelResolverStub) ResolveChannelMapping(context.Context, int64, string) service.ChannelMappingResult {
	return s.result
}

func TestOpenAIResponsesPlannerBuildsSerializablePlan(t *testing.T) {
	groupID := int64(42)
	planner := OpenAIResponsesPlanner{
		Channels: channelResolverStub{result: service.ChannelMappingResult{
			ChannelID:          101,
			MappedModel:        "gpt-5.4-upstream",
			Mapped:             true,
			BillingModelSource: service.BillingModelSourceChannelMapped,
		}},
	}
	req := &core.CanonicalRequest{
		RequestID:      "req_123",
		Endpoint:       core.EndpointResponses,
		Provider:       service.PlatformOpenAI,
		RequestedModel: "gpt-5.4",
		Body:           []byte(`{"model":"gpt-5.4","stream":false}`),
		Headers: http.Header{
			"Authorization": []string{"Bearer secret"},
			"Content-Type":  []string{"application/json"},
		},
		Session: core.SessionInput{Key: "sticky"},
	}
	apiKey := &service.APIKey{ID: 7, UserID: 8, GroupID: &groupID}

	plan, err := planner.Build(context.Background(), req, apiKey)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if plan.Model.RequestedModel != "gpt-5.4" {
		t.Fatalf("requested model = %q", plan.Model.RequestedModel)
	}
	if plan.Model.UpstreamModel != "gpt-5.4-upstream" {
		t.Fatalf("upstream model = %q", plan.Model.UpstreamModel)
	}
	if plan.Model.ChannelID != 101 {
		t.Fatalf("channel id = %d", plan.Model.ChannelID)
	}
	if plan.Billing.Model != "gpt-5.4-upstream" {
		t.Fatalf("billing model = %q", plan.Billing.Model)
	}
	if plan.Debug.HeaderPreview["Authorization"] != "Bearer [redacted]" {
		t.Fatalf("authorization header leaked: %q", plan.Debug.HeaderPreview["Authorization"])
	}

	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("plan should be serializable: %v", err)
	}
	const want = `"request_id":"req_123"`
	if !json.Valid(encoded) || !strings.Contains(string(encoded), want) {
		t.Fatalf("serialized plan missing %s: %s", want, encoded)
	}
}

func TestOpenAIResponsesPlannerRoutingCases(t *testing.T) {
	groupID := int64(42)
	tests := []struct {
		name          string
		endpoint      core.EndpointKind
		mapping       service.ChannelMappingResult
		wantUpstream  string
		wantBilling   string
		wantSource    string
		wantSticky    bool
		wantWebSocket bool
		wantChannelID int64
	}{
		{
			name:          "unmapped requested billing",
			endpoint:      core.EndpointResponses,
			mapping:       service.ChannelMappingResult{MappedModel: "gpt-5.4", BillingModelSource: service.BillingModelSourceRequested},
			wantUpstream:  "gpt-5.4",
			wantBilling:   "gpt-5.4",
			wantSource:    service.BillingModelSourceRequested,
			wantSticky:    true,
			wantChannelID: 0,
		},
		{
			name: "channel mapped billing",
			mapping: service.ChannelMappingResult{
				ChannelID:          99,
				MappedModel:        "gpt-5.4-channel",
				Mapped:             true,
				BillingModelSource: service.BillingModelSourceChannelMapped,
			},
			endpoint:      core.EndpointResponses,
			wantUpstream:  "gpt-5.4-channel",
			wantBilling:   "gpt-5.4-channel",
			wantSource:    service.BillingModelSourceChannelMapped,
			wantSticky:    true,
			wantChannelID: 99,
		},
		{
			name: "websocket upstream billing without sticky",
			mapping: service.ChannelMappingResult{
				MappedModel:        "gpt-5.4-upstream",
				Mapped:             true,
				BillingModelSource: service.BillingModelSourceUpstream,
			},
			endpoint:      core.EndpointResponsesWebSocket,
			wantUpstream:  "gpt-5.4-upstream",
			wantBilling:   "gpt-5.4-upstream",
			wantSource:    service.BillingModelSourceUpstream,
			wantWebSocket: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			planner := OpenAIResponsesPlanner{Channels: channelResolverStub{result: tt.mapping}}
			req := &core.CanonicalRequest{
				RequestID:      "req_123",
				Endpoint:       tt.endpoint,
				Provider:       service.PlatformOpenAI,
				RequestedModel: "gpt-5.4",
				Body:           []byte(`{"model":"gpt-5.4"}`),
				Headers:        http.Header{"Authorization": []string{"Bearer secret"}},
				Session:        core.SessionInput{Key: "sticky"},
			}
			if tt.wantWebSocket {
				req.Session.Key = ""
			}
			plan, err := planner.Build(context.Background(), req, &service.APIKey{GroupID: &groupID})
			if err != nil {
				t.Fatalf("Build returned error: %v", err)
			}
			if plan.Endpoint != tt.endpoint {
				t.Fatalf("endpoint = %q", plan.Endpoint)
			}
			if got := plan.Model.UpstreamModel; got != tt.wantUpstream {
				t.Fatalf("upstream model = %q, want %q", got, tt.wantUpstream)
			}
			if got := plan.Billing.Model; got != tt.wantBilling {
				t.Fatalf("billing model = %q, want %q", got, tt.wantBilling)
			}
			if got := plan.Billing.ModelSource; got != tt.wantSource {
				t.Fatalf("billing source = %q, want %q", got, tt.wantSource)
			}
			if plan.Session.StickyEligible != tt.wantSticky {
				t.Fatalf("sticky eligible = %v, want %v", plan.Session.StickyEligible, tt.wantSticky)
			}
			if plan.Model.ChannelID != tt.wantChannelID {
				t.Fatalf("channel id = %d, want %d", plan.Model.ChannelID, tt.wantChannelID)
			}
			if plan.Debug.HeaderPreview["Authorization"] != "Bearer [redacted]" {
				t.Fatalf("authorization leaked in debug plan: %+v", plan.Debug.HeaderPreview)
			}
			encoded, err := json.Marshal(plan)
			if err != nil || !json.Valid(encoded) {
				t.Fatalf("plan should be fixture-serializable: %v %s", err, encoded)
			}
		})
	}
}

func TestOpenAIResponsesPlannerRejectsWrongEndpoint(t *testing.T) {
	_, err := (OpenAIResponsesPlanner{}).Build(context.Background(), &core.CanonicalRequest{
		Endpoint:       core.EndpointMessages,
		RequestedModel: "gpt-5.4",
	}, nil)
	if err == nil {
		t.Fatalf("expected unsupported endpoint error")
	}
}
