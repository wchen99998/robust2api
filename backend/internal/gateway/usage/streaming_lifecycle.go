package usage

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type StreamingLifecycleParams struct {
	RequestID          string
	APIKey             *service.APIKey
	User               *service.User
	Account            *service.Account
	Subscription       *service.UserSubscription
	RequestPayloadHash string
	ReasoningEffort    *string
	ServiceTier        *string
	OccurredAt         time.Time
	Plan               *core.RoutingPlan
	RequestedModel     string
}

func BuildStreamingLifecycleInput(params StreamingLifecycleParams) *service.StreamingBillingLifecycleInput {
	return &service.StreamingBillingLifecycleInput{
		RequestID:          params.RequestID,
		APIKey:             params.APIKey,
		User:               params.User,
		Account:            params.Account,
		Subscription:       params.Subscription,
		Model:              lifecycleModel(params.Plan, params.RequestedModel),
		ServiceTier:        params.ServiceTier,
		ReasoningEffort:    params.ReasoningEffort,
		RequestPayloadHash: params.RequestPayloadHash,
		OccurredAt:         params.OccurredAt,
	}
}

func lifecycleModel(plan *core.RoutingPlan, requestedModel string) string {
	if requestedModel != "" {
		return requestedModel
	}
	if plan == nil {
		return ""
	}
	if plan.Model.BillingModel != "" {
		return plan.Model.BillingModel
	}
	if plan.Model.RequestedModel != "" {
		return plan.Model.RequestedModel
	}
	return plan.Model.UpstreamModel
}
