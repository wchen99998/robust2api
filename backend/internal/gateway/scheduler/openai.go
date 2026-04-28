package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
)

var ErrNoAvailableAccounts = errors.New("no available accounts")

type Account struct {
	Snapshot domain.AccountSnapshot
	Legacy   any
}

type ScheduleRequest struct {
	GroupID            int64
	SessionKey         string
	PreviousResponseID string
	RequestedModel     string
	RequiredTransport  domain.TransportKind
	ExcludedAccountIDs map[int64]struct{}
}

type ScheduleResult struct {
	Account     Account
	Layer       domain.AccountDecisionLayer
	Reservation Reservation
	WaitPlan    domain.AccountWaitPlan
	Diagnostics domain.CandidateDiagnostics
}

type Reservation struct {
	AccountID int64
	Acquired  bool
	Release   func()
}

type ScheduleOutcome struct {
	Success      bool
	FirstTokenMs *int
}

type Ports interface {
	LookupPreviousResponseAccount(ctx context.Context, groupID int64, previousResponseID string) (int64, bool, error)
	GetStickySessionAccount(ctx context.Context, groupID int64, sessionKey string) (int64, bool, error)
	BindStickySession(ctx context.Context, groupID int64, sessionKey string, accountID int64) error
	DeleteStickySession(ctx context.Context, groupID int64, sessionKey string) error
	RefreshStickySession(ctx context.Context, groupID int64, sessionKey string) error
	ListSchedulableOpenAIAccounts(ctx context.Context, groupID int64) ([]Account, error)
	GetAccount(ctx context.Context, accountID int64) (Account, bool, error)
	AcquireAccountSlot(ctx context.Context, account Account) (Reservation, error)
	WaitPlan(ctx context.Context, account Account) domain.AccountWaitPlan
	ReportResult(ctx context.Context, accountID int64, outcome ScheduleOutcome)
}

type OpenAIScheduler struct {
	ports Ports
}

func NewOpenAIScheduler(ports Ports) *OpenAIScheduler {
	return &OpenAIScheduler{ports: ports}
}

type NoAvailableAccountsError struct {
	Diagnostics domain.CandidateDiagnostics
}

func (e *NoAvailableAccountsError) Error() string {
	return ErrNoAvailableAccounts.Error()
}

func (e *NoAvailableAccountsError) Unwrap() error {
	return ErrNoAvailableAccounts
}

func (s *OpenAIScheduler) Select(ctx context.Context, req ScheduleRequest) (*ScheduleResult, error) {
	if s == nil || s.ports == nil {
		return nil, fmt.Errorf("openai scheduler unavailable: %w", ErrNoAvailableAccounts)
	}

	if req.RequiredTransport == "" {
		req.RequiredTransport = domain.TransportHTTP
	}

	diagnostics := domain.CandidateDiagnostics{
		RejectCount: make(map[domain.RejectionReason]int),
	}
	evaluated := make(map[int64]struct{})

	previousResponseID := strings.TrimSpace(req.PreviousResponseID)
	if previousResponseID != "" {
		accountID, ok, err := s.ports.LookupPreviousResponseAccount(ctx, req.GroupID, previousResponseID)
		if err != nil {
			return nil, err
		}
		if ok {
			result, selected, err := s.tryAccount(ctx, req, accountID, domain.AccountDecisionPreviousResponseID, &diagnostics, evaluated)
			if err != nil {
				return nil, err
			}
			if selected {
				return result, nil
			}
		}
	}

	sessionKey := strings.TrimSpace(req.SessionKey)
	if sessionKey != "" {
		accountID, ok, err := s.ports.GetStickySessionAccount(ctx, req.GroupID, sessionKey)
		if err != nil {
			return nil, err
		}
		if ok {
			result, selected, err := s.tryAccount(ctx, req, accountID, domain.AccountDecisionSessionHash, &diagnostics, evaluated)
			if err != nil {
				return nil, err
			}
			if selected {
				return result, nil
			}
			if err := s.ports.DeleteStickySession(ctx, req.GroupID, sessionKey); err != nil {
				return nil, err
			}
		}
	}

	accounts, err := s.ports.ListSchedulableOpenAIAccounts(ctx, req.GroupID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(accounts, func(i, j int) bool {
		left := accounts[i].Snapshot
		right := accounts[j].Snapshot
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		return left.ID < right.ID
	})

	var firstWaitResult *ScheduleResult
	for _, account := range accounts {
		if !markEvaluated(&diagnostics, evaluated, account.Snapshot.ID) {
			continue
		}
		if reason, ok := eligible(account, req); !ok {
			reject(&diagnostics, account.Snapshot.ID, reason)
			continue
		}
		result, err := s.reserve(ctx, req, account, domain.AccountDecisionLoadBalance, &diagnostics)
		if err != nil {
			return nil, err
		}
		if result.Reservation.Acquired {
			return result, nil
		}
		if firstWaitResult == nil {
			firstWaitResult = result
		}
	}

	if firstWaitResult != nil {
		diagnostics.Rejected = sumRejects(diagnostics.RejectCount)
		firstWaitResult.Diagnostics = diagnostics
		return firstWaitResult, nil
	}

	diagnostics.Rejected = sumRejects(diagnostics.RejectCount)
	return nil, &NoAvailableAccountsError{Diagnostics: diagnostics}
}

func (s *OpenAIScheduler) ReportResult(ctx context.Context, accountID int64, outcome ScheduleOutcome) {
	if s == nil || s.ports == nil {
		return
	}
	s.ports.ReportResult(ctx, accountID, outcome)
}

func (s *OpenAIScheduler) tryAccount(
	ctx context.Context,
	req ScheduleRequest,
	accountID int64,
	layer domain.AccountDecisionLayer,
	diagnostics *domain.CandidateDiagnostics,
	evaluated map[int64]struct{},
) (*ScheduleResult, bool, error) {
	if !markEvaluated(diagnostics, evaluated, accountID) {
		return nil, false, nil
	}

	account, ok, err := s.ports.GetAccount(ctx, accountID)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		reject(diagnostics, accountID, domain.RejectionReasonUnschedulable)
		return nil, false, nil
	}
	if reason, ok := eligible(account, req); !ok {
		reject(diagnostics, account.Snapshot.ID, reason)
		return nil, false, nil
	}

	returnResult, err := s.reserve(ctx, req, account, layer, diagnostics)
	if err != nil {
		return nil, false, err
	}
	return returnResult, true, nil
}

func (s *OpenAIScheduler) reserve(
	ctx context.Context,
	req ScheduleRequest,
	account Account,
	layer domain.AccountDecisionLayer,
	diagnostics *domain.CandidateDiagnostics,
) (*ScheduleResult, error) {
	diagnostics.Eligible++
	diagnostics.Rejected = sumRejects(diagnostics.RejectCount)

	reservation, err := s.ports.AcquireAccountSlot(ctx, account)
	if err != nil {
		return nil, err
	}
	if reservation.AccountID == 0 {
		reservation.AccountID = account.Snapshot.ID
	}

	waitPlan := domain.AccountWaitPlan{}
	if reservation.Acquired {
		sessionKey := strings.TrimSpace(req.SessionKey)
		if sessionKey != "" {
			if err := s.ports.BindStickySession(ctx, req.GroupID, sessionKey, account.Snapshot.ID); err != nil {
				if reservation.Release != nil {
					reservation.Release()
				}
				return nil, err
			}
		}
	} else {
		waitPlan = s.ports.WaitPlan(ctx, account)
		if !waitPlan.Required {
			waitPlan = domain.AccountWaitPlan{
				Required: true,
				Reason:   "account_busy",
				Timeout:  30 * time.Second,
			}
		}
	}

	return &ScheduleResult{
		Account:     account,
		Layer:       layer,
		Reservation: reservation,
		WaitPlan:    waitPlan,
		Diagnostics: *diagnostics,
	}, nil
}

func markEvaluated(diagnostics *domain.CandidateDiagnostics, evaluated map[int64]struct{}, accountID int64) bool {
	if _, ok := evaluated[accountID]; ok {
		return false
	}
	evaluated[accountID] = struct{}{}
	diagnostics.Total++
	return true
}

func eligible(account Account, req ScheduleRequest) (domain.RejectionReason, bool) {
	if _, excluded := req.ExcludedAccountIDs[account.Snapshot.ID]; excluded {
		return domain.RejectionReasonExcluded, false
	}
	if account.Snapshot.Platform != domain.PlatformOpenAI {
		return domain.RejectionReasonPlatformMismatch, false
	}
	if !supportsModel(account.Snapshot.Platform, account.Snapshot.Capabilities.Models, req.RequestedModel) {
		return domain.RejectionReasonModelUnsupported, false
	}
	if !supportsTransport(account.Snapshot.Capabilities.Transports, req.RequiredTransport) {
		return domain.RejectionReasonTransportMismatch, false
	}
	return "", true
}

func supportsModel(platform domain.Platform, models []string, requestedModel string) bool {
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" || len(models) == 0 {
		return true
	}
	if supportsModelName(models, requestedModel) {
		return true
	}
	normalized := normalizeRequestedModelForLookup(platform, requestedModel)
	return normalized != requestedModel && supportsModelName(models, normalized)
}

func supportsModelName(models []string, requestedModel string) bool {
	for _, model := range models {
		model = strings.TrimSpace(model)
		if strings.EqualFold(model, requestedModel) {
			return true
		}
		if supportsModelWildcard(model, requestedModel) {
			return true
		}
	}
	return false
}

func supportsModelWildcard(pattern string, requestedModel string) bool {
	if !strings.HasSuffix(pattern, "*") {
		return false
	}
	prefix := pattern[:len(pattern)-1]
	return strings.HasPrefix(requestedModel, prefix)
}

func normalizeRequestedModelForLookup(platform domain.Platform, requestedModel string) string {
	requestedModel = strings.TrimSpace(requestedModel)
	if platform != domain.PlatformGemini && platform != domain.PlatformAntigravity {
		return requestedModel
	}
	if requestedModel == "gemini-3.1-pro-preview-customtools" {
		return "gemini-3.1-pro-preview"
	}
	return requestedModel
}

func supportsTransport(transports []domain.TransportKind, requiredTransport domain.TransportKind) bool {
	if requiredTransport == "" {
		requiredTransport = domain.TransportHTTP
	}
	if requiredTransport == domain.TransportHTTP {
		if len(transports) == 0 {
			return true
		}
		for _, transport := range transports {
			if transport == "" || transport == domain.TransportHTTP {
				return true
			}
		}
		return false
	}
	for _, transport := range transports {
		if transport == requiredTransport {
			return true
		}
	}
	return false
}

func reject(diagnostics *domain.CandidateDiagnostics, accountID int64, reason domain.RejectionReason) {
	if diagnostics.RejectCount == nil {
		diagnostics.RejectCount = make(map[domain.RejectionReason]int)
	}
	diagnostics.RejectCount[reason]++
	diagnostics.Samples = append(diagnostics.Samples, domain.CandidateSample{
		AccountID: accountID,
		Reason:    reason,
	})
}

func sumRejects(rejects map[domain.RejectionReason]int) int {
	total := 0
	for _, count := range rejects {
		total += count
	}
	return total
}
