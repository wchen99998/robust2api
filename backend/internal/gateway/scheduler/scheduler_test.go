package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type fakeOpenAIResponsesScheduleService struct {
	selection           *service.AccountSelectionResult
	decision            service.OpenAIAccountScheduleDecision
	err                 error
	previousResponseID  string
	sessionHash         string
	requestedModel      string
	requiredTransport   service.OpenAIUpstreamTransport
	excludedAccountIDOK bool
}

func (s *fakeOpenAIResponsesScheduleService) SelectAccountWithScheduler(
	_ context.Context,
	_ *int64,
	previousResponseID string,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	requiredTransport service.OpenAIUpstreamTransport,
) (*service.AccountSelectionResult, service.OpenAIAccountScheduleDecision, error) {
	s.previousResponseID = previousResponseID
	s.sessionHash = sessionHash
	s.requestedModel = requestedModel
	s.requiredTransport = requiredTransport
	_, s.excludedAccountIDOK = excludedIDs[99]
	return s.selection, s.decision, s.err
}

func TestSelectReturnsDeterministicPriorityLRUAccount(t *testing.T) {
	now := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	newer := now.Add(-time.Minute)
	older := now.Add(-time.Hour)
	plan := core.RoutingPlan{
		Provider: service.PlatformOpenAI,
		Model:    core.ModelResolution{UpstreamModel: "gpt-5.4"},
	}
	result, err := Select(SelectionInput{
		Plan: plan,
		Accounts: []service.Account{
			{ID: 1, Platform: service.PlatformOpenAI, Status: service.StatusActive, Schedulable: true, Priority: 1, LastUsedAt: &newer},
			{ID: 2, Platform: service.PlatformOpenAI, Status: service.StatusActive, Schedulable: true, Priority: 1, LastUsedAt: &older},
			{ID: 3, Platform: service.PlatformAnthropic, Status: service.StatusActive, Schedulable: true, Priority: 0},
		},
		Now: now,
	})
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if result.Account.ID != 2 {
		t.Fatalf("selected account = %d, want 2", result.Account.ID)
	}
	if result.Plan.Account.AccountID != 2 {
		t.Fatalf("plan account = %d, want 2", result.Plan.Account.AccountID)
	}
	if result.Diagnostic.Rejected[core.RejectedPlatformMismatch] != 1 {
		t.Fatalf("platform mismatch count = %d", result.Diagnostic.Rejected[core.RejectedPlatformMismatch])
	}
}

func TestSelectReturnsStructuredRejectionsWhenNoEligibleAccount(t *testing.T) {
	resetAt := time.Date(2026, 4, 25, 1, 0, 0, 0, time.UTC)
	plan := core.RoutingPlan{
		Provider: service.PlatformOpenAI,
		Model:    core.ModelResolution{UpstreamModel: "gpt-5.4"},
	}
	result, err := Select(SelectionInput{
		Plan: plan,
		Accounts: []service.Account{
			{ID: 1, Platform: service.PlatformAnthropic, Status: service.StatusActive, Schedulable: true},
			{ID: 2, Platform: service.PlatformOpenAI, Status: service.StatusDisabled, Schedulable: true},
			{ID: 3, Platform: service.PlatformOpenAI, Status: service.StatusActive, Schedulable: true, RateLimitResetAt: &resetAt},
		},
		Now: resetAt.Add(-time.Minute),
	})
	if err == nil {
		t.Fatalf("expected no eligible accounts error")
	}
	if result.Diagnostic.Rejected[core.RejectedPlatformMismatch] != 1 {
		t.Fatalf("platform mismatch count = %d", result.Diagnostic.Rejected[core.RejectedPlatformMismatch])
	}
	if result.Diagnostic.Rejected[core.RejectedUnschedulable] != 1 {
		t.Fatalf("unschedulable count = %d", result.Diagnostic.Rejected[core.RejectedUnschedulable])
	}
	if result.Diagnostic.Rejected[core.RejectedRPMLimited] != 1 {
		t.Fatalf("rpm count = %d", result.Diagnostic.Rejected[core.RejectedRPMLimited])
	}
}

func TestOpenAIResponsesSelectorAnnotatesRoutingPlan(t *testing.T) {
	fake := &fakeOpenAIResponsesScheduleService{
		selection: &service.AccountSelectionResult{
			Account:     &service.Account{ID: 42, Name: "openai-primary", Platform: service.PlatformOpenAI},
			Acquired:    true,
			ReleaseFunc: func() {},
			WaitPlan:    &service.AccountWaitPlan{AccountID: 42},
		},
		decision: service.OpenAIAccountScheduleDecision{
			Layer:             "session_hash",
			StickySessionHit:  true,
			CandidateCount:    7,
			TopK:              3,
			LatencyMs:         11,
			LoadSkew:          0.25,
			SelectedAccountID: 42,
		},
	}
	selector := OpenAIResponsesSelector{
		Service:            fake,
		PreviousResponseID: "resp_123",
		FailedAccountIDs:   map[int64]struct{}{99: struct{}{}},
		RequiredTransport:  service.OpenAIUpstreamTransportResponsesWebsocketV2,
	}
	plan := core.RoutingPlan{
		Session: core.SessionDecision{Key: "session-hash"},
		Model:   core.ModelResolution{RequestedModel: "gpt-5.4", UpstreamModel: "gpt-5.4-upstream"},
	}

	result, err := selector.SelectOpenAIResponses(context.Background(), plan)
	if err != nil {
		t.Fatalf("SelectOpenAIResponses returned error: %v", err)
	}
	if fake.previousResponseID != "resp_123" || fake.sessionHash != "session-hash" {
		t.Fatalf("unexpected sticky inputs: previous=%q session=%q", fake.previousResponseID, fake.sessionHash)
	}
	if fake.requestedModel != "gpt-5.4" {
		t.Fatalf("requested model = %q", fake.requestedModel)
	}
	if fake.requiredTransport != service.OpenAIUpstreamTransportResponsesWebsocketV2 {
		t.Fatalf("required transport = %q", fake.requiredTransport)
	}
	if !fake.excludedAccountIDOK {
		t.Fatalf("excluded account IDs were not passed through")
	}
	if result.Plan.Account.AccountID != 42 || result.Plan.Account.SelectionMode != "session_hash" {
		t.Fatalf("unexpected plan account: %+v", result.Plan.Account)
	}
	if !result.Plan.Account.StickySelected || !result.Plan.Account.Acquired || !result.Plan.Account.WaitAllowed {
		t.Fatalf("unexpected plan account flags: %+v", result.Plan.Account)
	}
	if result.Plan.Candidates.Total != 7 || result.Plan.Candidates.TopK != 3 || result.Plan.Candidates.LatencyMs != 11 {
		t.Fatalf("unexpected candidate diagnostics: %+v", result.Plan.Candidates)
	}
}

func TestOpenAIResponsesSelectorReturnsPlanOnServiceError(t *testing.T) {
	fake := &fakeOpenAIResponsesScheduleService{
		decision: service.OpenAIAccountScheduleDecision{CandidateCount: 2},
		err:      errors.New("no account"),
	}
	selector := OpenAIResponsesSelector{Service: fake}

	result, err := selector.Select(context.Background(), core.RoutingPlan{Model: core.ModelResolution{RequestedModel: "gpt-5.4"}})
	if err == nil {
		t.Fatalf("expected error")
	}
	if result == nil || result.Plan.Candidates.Total != 2 {
		t.Fatalf("expected annotated plan on error, got %+v", result)
	}
}
