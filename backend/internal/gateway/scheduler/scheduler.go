package scheduler

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type SelectionInput struct {
	Plan        core.RoutingPlan
	Accounts    []service.Account
	ExcludedIDs map[int64]struct{}
	Now         time.Time
}

type SelectionResult struct {
	Plan       core.RoutingPlan
	Account    *service.Account
	Diagnostic core.CandidateDiagnostics
}

func Select(input SelectionInput) (*SelectionResult, error) {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	diagnostic := core.CandidateDiagnostics{
		RequestedPlatform: input.Plan.Provider,
		Total:             len(input.Accounts),
		Rejected:          map[core.RejectionReason]int{},
		Samples:           map[core.RejectionReason][]int64{},
	}

	eligible := make([]service.Account, 0, len(input.Accounts))
	for _, account := range input.Accounts {
		if reason, rejected := rejectAccount(input.Plan, account, input.ExcludedIDs, now); rejected {
			addRejection(&diagnostic, reason, account.ID)
			continue
		}
		eligible = append(eligible, account)
	}
	diagnostic.Eligible = len(eligible)

	plan := input.Plan
	plan.Candidates = diagnostic
	if len(eligible) == 0 {
		return &SelectionResult{Plan: plan, Diagnostic: diagnostic}, errors.New("no eligible accounts")
	}

	sort.SliceStable(eligible, func(i, j int) bool {
		if eligible[i].Priority != eligible[j].Priority {
			return eligible[i].Priority < eligible[j].Priority
		}
		if eligible[i].LastUsedAt == nil && eligible[j].LastUsedAt != nil {
			return true
		}
		if eligible[i].LastUsedAt != nil && eligible[j].LastUsedAt == nil {
			return false
		}
		if eligible[i].LastUsedAt != nil && eligible[j].LastUsedAt != nil && !eligible[i].LastUsedAt.Equal(*eligible[j].LastUsedAt) {
			return eligible[i].LastUsedAt.Before(*eligible[j].LastUsedAt)
		}
		return eligible[i].ID < eligible[j].ID
	})

	selected := eligible[0]
	plan.Account = core.AccountDecision{
		AccountID:     selected.ID,
		AccountName:   selected.Name,
		Platform:      selected.Platform,
		SelectionMode: "priority_lru",
	}
	return &SelectionResult{Plan: plan, Account: &selected, Diagnostic: diagnostic}, nil
}

func rejectAccount(plan core.RoutingPlan, account service.Account, excluded map[int64]struct{}, now time.Time) (core.RejectionReason, bool) {
	if _, ok := excluded[account.ID]; ok {
		return core.RejectedExcluded, true
	}
	if account.Platform != plan.Provider {
		return core.RejectedPlatformMismatch, true
	}
	if account.Status != service.StatusActive || !account.Schedulable {
		return core.RejectedUnschedulable, true
	}
	if account.ExpiresAt != nil && now.After(*account.ExpiresAt) {
		return core.RejectedUnschedulable, true
	}
	if account.TempUnschedulableUntil != nil && now.Before(*account.TempUnschedulableUntil) {
		return core.RejectedUnschedulable, true
	}
	if account.RateLimitResetAt != nil && now.Before(*account.RateLimitResetAt) {
		return core.RejectedRPMLimited, true
	}
	if plan.Model.UpstreamModel != "" && !account.IsModelSupported(plan.Model.UpstreamModel) {
		return core.RejectedModelUnsupported, true
	}
	return "", false
}

func addRejection(d *core.CandidateDiagnostics, reason core.RejectionReason, accountID int64) {
	d.Rejected[reason]++
	if len(d.Samples[reason]) < 5 {
		d.Samples[reason] = append(d.Samples[reason], accountID)
	}
}

type OpenAIResponsesScheduleService interface {
	SelectAccountWithScheduler(
		ctx context.Context,
		groupID *int64,
		previousResponseID string,
		sessionHash string,
		requestedModel string,
		excludedIDs map[int64]struct{},
		requiredTransport service.OpenAIUpstreamTransport,
	) (*service.AccountSelectionResult, service.OpenAIAccountScheduleDecision, error)
}

type OpenAIResponsesSelection struct {
	Plan             *core.RoutingPlan
	ServiceSelection *service.AccountSelectionResult
	ScheduleDecision service.OpenAIAccountScheduleDecision
}

type OpenAIResponsesSelector struct {
	Service            OpenAIResponsesScheduleService
	PreviousResponseID string
	FailedAccountIDs   map[int64]struct{}
	RequiredTransport  service.OpenAIUpstreamTransport
}

func (s OpenAIResponsesSelector) Select(ctx context.Context, plan core.RoutingPlan) (*core.AccountSelection, error) {
	selection, err := s.SelectOpenAIResponses(ctx, plan)
	if err != nil {
		if selection != nil && selection.Plan != nil {
			return &core.AccountSelection{Plan: *selection.Plan}, err
		}
		return nil, err
	}
	if selection == nil || selection.ServiceSelection == nil {
		return nil, nil
	}
	return &core.AccountSelection{
		Plan:    *selection.Plan,
		Account: selection.ServiceSelection.Account,
	}, nil
}

func (s OpenAIResponsesSelector) SelectOpenAIResponses(ctx context.Context, plan core.RoutingPlan) (*OpenAIResponsesSelection, error) {
	if s.Service == nil {
		return nil, errors.New("openai responses schedule service is required")
	}
	model := plan.Model.RequestedModel
	if model == "" {
		model = plan.Model.UpstreamModel
	}
	requiredTransport := s.RequiredTransport
	if requiredTransport == "" {
		requiredTransport = service.OpenAIUpstreamTransportAny
	}
	selection, decision, err := s.Service.SelectAccountWithScheduler(
		ctx,
		plan.GroupID,
		s.PreviousResponseID,
		plan.Session.Key,
		model,
		s.FailedAccountIDs,
		requiredTransport,
	)
	annotateOpenAIResponsesPlan(&plan, selection, decision)
	return &OpenAIResponsesSelection{
		Plan:             &plan,
		ServiceSelection: selection,
		ScheduleDecision: decision,
	}, err
}

func annotateOpenAIResponsesPlan(plan *core.RoutingPlan, selection *service.AccountSelectionResult, decision service.OpenAIAccountScheduleDecision) {
	if plan == nil {
		return
	}
	plan.Candidates.Total = decision.CandidateCount
	plan.Candidates.Eligible = decision.CandidateCount
	plan.Candidates.TopK = decision.TopK
	plan.Candidates.LatencyMs = decision.LatencyMs
	plan.Candidates.LoadSkew = decision.LoadSkew
	plan.Account.SelectionMode = decision.Layer
	plan.Account.StickySelected = decision.StickyPreviousHit || decision.StickySessionHit
	if selection != nil {
		plan.Account.Acquired = selection.Acquired
		plan.Account.WaitAllowed = selection.WaitPlan != nil
		if selection.Account != nil {
			plan.Account.AccountID = selection.Account.ID
			plan.Account.AccountName = selection.Account.Name
			plan.Account.Platform = selection.Account.Platform
		}
	}
	if decision.SelectedAccountID > 0 && plan.Account.AccountID == 0 {
		plan.Account.AccountID = decision.SelectedAccountID
	}
}
