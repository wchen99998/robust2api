package openai

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestAdapterParseResponsesRequest(t *testing.T) {
	req := core.IngressRequest{
		RequestID: "req_123",
		Method:    http.MethodPost,
		Path:      "/openai/v1/responses",
		Headers:   http.Header{"User-Agent": []string{"test"}},
		Body:      []byte(`{"model":"gpt-5.4","stream":true}`),
		ClientIP:  "1.2.3.4",
	}
	parsed, err := (Adapter{}).Parse(context.Background(), req)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if parsed.Provider != service.PlatformOpenAI {
		t.Fatalf("provider = %q", parsed.Provider)
	}
	if parsed.RequestedModel != "gpt-5.4" {
		t.Fatalf("model = %q", parsed.RequestedModel)
	}
	if !parsed.Stream {
		t.Fatalf("stream should be true")
	}
}

func TestAdapterPrepareUsesCustomBaseURL(t *testing.T) {
	account := &service.Account{
		ID:          1,
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://relay.example.com"},
	}
	plan := core.RoutingPlan{
		Endpoint: core.EndpointResponses,
		Model:    core.ModelResolution{UpstreamModel: "gpt-5.4-upstream"},
	}
	req, err := (Adapter{}).Prepare(context.Background(), plan, account)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
	if req.URL != "https://relay.example.com/v1/responses" {
		t.Fatalf("URL = %q", req.URL)
	}
	if req.Headers.Get("x-sub2api-upstream-model") != "gpt-5.4-upstream" {
		t.Fatalf("upstream model header = %q", req.Headers.Get("x-sub2api-upstream-model"))
	}
}

func TestPrepareResponsesBodyAppliesChannelMappedModel(t *testing.T) {
	plan := core.RoutingPlan{
		Model: core.ModelResolution{
			RequestedModel:     "gpt-5.4",
			ChannelMappedModel: "gpt-5.4-upstream",
		},
	}

	body := PrepareResponsesBody(plan, []byte(`{"input":"hello","model":"gpt-5.4"}`))

	if string(body) != `{"input":"hello","model":"gpt-5.4-upstream"}` {
		t.Fatalf("body = %s", body)
	}
}

func TestPrepareResponsesBodyCopiesUnmappedBody(t *testing.T) {
	original := []byte(`{"model":"gpt-5.4"}`)
	body := PrepareResponsesBody(core.RoutingPlan{Model: core.ModelResolution{RequestedModel: "gpt-5.4", ChannelMappedModel: "gpt-5.4"}}, original)
	original[0] = '['

	if string(body) != `{"model":"gpt-5.4"}` {
		t.Fatalf("body = %s", body)
	}
}
