package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type Planner interface {
	Build(ctx context.Context, req *CanonicalRequest, apiKey *service.APIKey) (*RoutingPlan, error)
}

type AccountSelection struct {
	Plan    RoutingPlan
	Account *service.Account
}

type AccountSelector interface {
	Select(ctx context.Context, plan RoutingPlan) (*AccountSelection, error)
}

type Transport interface {
	RoundTrip(ctx context.Context, req *UpstreamRequest) (*UpstreamResult, error)
}

type UsageMapper interface {
	Map(ctx context.Context, plan RoutingPlan, account *service.Account, result *GatewayResult) (*UsageEvent, error)
}

type Pipeline struct {
	Adapter   ProviderAdapter
	Planner   Planner
	Selector  AccountSelector
	Transport Transport
	Usage     UsageMapper
}

func (p Pipeline) Handle(ctx context.Context, req IngressRequest) (*GatewayResult, error) {
	if p.Adapter == nil {
		return nil, errors.New("gateway provider adapter is required")
	}
	if p.Planner == nil {
		return nil, errors.New("gateway planner is required")
	}
	if p.Selector == nil {
		return nil, errors.New("gateway account selector is required")
	}
	if p.Transport == nil {
		return nil, errors.New("gateway transport is required")
	}

	canonical, err := p.Adapter.Parse(ctx, req)
	if err != nil {
		return nil, err
	}
	plan, err := p.Planner.Build(ctx, canonical, req.APIKey)
	if err != nil {
		return nil, err
	}
	selection, err := p.Selector.Select(ctx, *plan)
	if err != nil {
		plan.Candidates = mergeCandidateDiagnostics(plan.Candidates, selection)
		return &GatewayResult{
			RequestID: plan.RequestID,
			Status:    http.StatusServiceUnavailable,
			Headers:   http.Header{"Content-Type": []string{"application/json"}},
			Body:      []byte(`{"error":{"type":"api_error","message":"No available accounts"}}`),
			Plan:      plan,
		}, err
	}
	if selection == nil || selection.Account == nil {
		return nil, errors.New("gateway account selector returned no account")
	}
	plan = &selection.Plan
	upstreamReq, err := p.Adapter.Prepare(ctx, *plan, selection.Account)
	if err != nil {
		return nil, err
	}
	if upstreamReq.Body == nil {
		upstreamReq.Body = append([]byte(nil), canonical.Body...)
	}
	upstreamResult, err := p.Transport.RoundTrip(ctx, upstreamReq)
	if err != nil {
		decision := p.Adapter.ClassifyError(ctx, nil)
		return &GatewayResult{
			RequestID: plan.RequestID,
			Status:    nonZeroStatus(decision.ClientStatus, http.StatusBadGateway),
			Headers:   http.Header{"Content-Type": []string{"application/json"}},
			Body:      []byte(fmt.Sprintf(`{"error":{"type":%q,"message":%q}}`, decision.ClientErrorType, decision.ClientErrorMessage)),
			Plan:      plan,
		}, err
	}
	if upstreamResult.StatusCode >= 400 {
		decision := p.Adapter.ClassifyError(ctx, upstreamResult)
		return &GatewayResult{
			RequestID: plan.RequestID,
			Status:    nonZeroStatus(decision.ClientStatus, upstreamResult.StatusCode),
			Headers:   upstreamResult.Headers.Clone(),
			Body:      append([]byte(nil), upstreamResult.Body...),
			Plan:      plan,
		}, nil
	}
	result, err := p.Adapter.Decode(ctx, upstreamResult)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("gateway provider adapter returned no result")
	}
	result.RequestID = plan.RequestID
	result.Plan = plan
	if p.Usage != nil {
		usage, err := p.Usage.Map(ctx, *plan, selection.Account, result)
		if err != nil {
			return nil, err
		}
		result.Usage = usage
	}
	return result, nil
}

func mergeCandidateDiagnostics(current CandidateDiagnostics, selection *AccountSelection) CandidateDiagnostics {
	if selection == nil {
		return current
	}
	if selection.Plan.Candidates.Total != 0 || selection.Plan.Candidates.Eligible != 0 || len(selection.Plan.Candidates.Rejected) > 0 {
		return selection.Plan.Candidates
	}
	return current
}

func nonZeroStatus(status, fallback int) int {
	if status != 0 {
		return status
	}
	return fallback
}
