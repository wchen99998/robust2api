package planning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/gateway/observability"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type ChannelResolver interface {
	ResolveChannelMapping(ctx context.Context, groupID int64, model string) service.ChannelMappingResult
}

type OpenAIResponsesPlanner struct {
	Channels ChannelResolver
}

func (p OpenAIResponsesPlanner) Build(ctx context.Context, req *core.CanonicalRequest, apiKey *service.APIKey) (*core.RoutingPlan, error) {
	if req == nil {
		return nil, errors.New("canonical request is required")
	}
	if req.Endpoint != core.EndpointResponses && req.Endpoint != core.EndpointResponsesWebSocket {
		return nil, fmt.Errorf("unsupported endpoint for OpenAI responses planner: %s", req.Endpoint)
	}
	if req.RequestedModel == "" {
		return nil, errors.New("requested model is required")
	}

	model := core.ModelResolution{
		RequestedModel:     req.RequestedModel,
		ChannelMappedModel: req.RequestedModel,
		UpstreamModel:      req.RequestedModel,
		BillingModel:       req.RequestedModel,
		BillingModelSource: service.BillingModelSourceRequested,
	}
	var groupID *int64
	if apiKey != nil {
		groupID = apiKey.GroupID
	}
	if p.Channels != nil && groupID != nil {
		mapping := p.Channels.ResolveChannelMapping(ctx, *groupID, req.RequestedModel)
		model.ChannelID = mapping.ChannelID
		if mapping.MappedModel != "" {
			model.ChannelMappedModel = mapping.MappedModel
			model.UpstreamModel = mapping.MappedModel
		}
		model.BillingModelSource = mapping.BillingModelSource
		model.BillingModel = billingModelForSource(mapping.BillingModelSource, req.RequestedModel, model.ChannelMappedModel, model.UpstreamModel)
	}

	session := core.SessionDecision{
		Key:            req.Session.Key,
		StickyEligible: req.Session.Key != "",
		Source:         "request",
	}
	if session.Key == "" {
		session.Source = "none"
	}

	payloadHash := payloadFingerprint(req.Body)
	return &core.RoutingPlan{
		RequestID: req.RequestID,
		Endpoint:  req.Endpoint,
		Provider:  service.PlatformOpenAI,
		GroupID:   groupID,
		Model:     model,
		Session:   session,
		Candidates: core.CandidateDiagnostics{
			RequestedPlatform: service.PlatformOpenAI,
		},
		Retry: core.RetryPlan{
			MaxAttempts: 3,
			SameAccount: true,
		},
		Billing: core.BillingPlan{
			Enabled:            true,
			Model:              model.BillingModel,
			ModelSource:        model.BillingModelSource,
			IdempotencyKey:     req.RequestID,
			PayloadFingerprint: payloadHash,
		},
		Debug: core.DebugPlan{
			HeaderPreview:   observability.SafeHeaders(req.Headers),
			BodyFingerprint: observability.BodyFingerprint(req.Body),
		},
	}, nil
}

func billingModelForSource(source, requestedModel, channelMappedModel, upstreamModel string) string {
	switch source {
	case service.BillingModelSourceRequested:
		return requestedModel
	case service.BillingModelSourceUpstream:
		if upstreamModel != "" {
			return upstreamModel
		}
		return requestedModel
	case service.BillingModelSourceChannelMapped, "":
		if channelMappedModel != "" {
			return channelMappedModel
		}
		return requestedModel
	default:
		if channelMappedModel != "" {
			return channelMappedModel
		}
		return requestedModel
	}
}

func payloadFingerprint(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
