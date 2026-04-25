package usage

import (
	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type OpenAIRecordUsageParams struct {
	Result             *service.OpenAIForwardResult
	APIKey             *service.APIKey
	User               *service.User
	Account            *service.Account
	Subscription       *service.UserSubscription
	BillingRequestID   string
	BillingEventKind   service.UsageChargeEventKind
	InboundEndpoint    string
	UpstreamEndpoint   string
	UserAgent          string
	IPAddress          string
	RequestPayloadHash string
	APIKeyService      service.APIKeyQuotaUpdater
	Plan               *core.RoutingPlan
	RequestedModel     string
}

func BuildOpenAIRecordUsageInput(params OpenAIRecordUsageParams) *service.OpenAIRecordUsageInput {
	return &service.OpenAIRecordUsageInput{
		Result:             params.Result,
		APIKey:             params.APIKey,
		User:               params.User,
		Account:            params.Account,
		Subscription:       params.Subscription,
		BillingRequestID:   params.BillingRequestID,
		BillingEventKind:   params.BillingEventKind,
		InboundEndpoint:    params.InboundEndpoint,
		UpstreamEndpoint:   params.UpstreamEndpoint,
		UserAgent:          params.UserAgent,
		IPAddress:          params.IPAddress,
		RequestPayloadHash: params.RequestPayloadHash,
		APIKeyService:      params.APIKeyService,
		ChannelUsageFields: channelUsageFields(params.Plan, params.Result, params.RequestedModel),
	}
}

func channelUsageFields(plan *core.RoutingPlan, result *service.OpenAIForwardResult, requestedModelOverride string) service.ChannelUsageFields {
	if plan == nil {
		return service.ChannelUsageFields{}
	}
	requestedModel := plan.Model.RequestedModel
	if requestedModelOverride != "" {
		requestedModel = requestedModelOverride
	}
	channelMappedModel := plan.Model.ChannelMappedModel
	if channelMappedModel == "" {
		channelMappedModel = requestedModel
	}
	upstreamModel := plan.Model.UpstreamModel
	if result != nil && result.UpstreamModel != "" {
		upstreamModel = result.UpstreamModel
	}
	return service.ChannelUsageFields{
		ChannelID:          plan.Model.ChannelID,
		OriginalModel:      requestedModel,
		ChannelMappedModel: channelMappedModel,
		BillingModelSource: plan.Model.BillingModelSource,
		ModelMappingChain:  buildModelMappingChain(requestedModel, channelMappedModel, upstreamModel),
	}
}

func buildModelMappingChain(requestedModel, channelMappedModel, upstreamModel string) string {
	if requestedModel == "" {
		return ""
	}
	mapped := channelMappedModel
	if mapped == "" {
		mapped = requestedModel
	}
	if mapped == requestedModel {
		if upstreamModel != "" && upstreamModel != requestedModel {
			return requestedModel + "→" + upstreamModel
		}
		return ""
	}
	if upstreamModel != "" && upstreamModel != mapped {
		return requestedModel + "→" + mapped + "→" + upstreamModel
	}
	return requestedModel + "→" + mapped
}
