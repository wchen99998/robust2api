package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
)

func TestOpenAISchedulerPreviousResponseStickyWins(t *testing.T) {
	ports := newFakePorts()
	ports.previous["resp_1"] = 10
	ports.accounts[10] = testAccount(10, string(domain.AccountTypeAPIKey), "gpt-5.1", domain.TransportWebSocket)
	ports.accounts[20] = testAccount(20, string(domain.AccountTypeAPIKey), "gpt-5.1", domain.TransportWebSocket)

	result, err := NewOpenAIScheduler(ports).Select(context.Background(), ScheduleRequest{
		GroupID:            1,
		PreviousResponseID: "resp_1",
		RequestedModel:     "gpt-5.1",
		RequiredTransport:  domain.TransportWebSocket,
	})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}

	if result.Account.Snapshot.ID != 10 {
		t.Fatalf("account ID = %d, want 10", result.Account.Snapshot.ID)
	}
	if result.Layer != domain.AccountDecisionPreviousResponseID {
		t.Fatalf("layer = %q, want %q", result.Layer, domain.AccountDecisionPreviousResponseID)
	}
	if !result.Reservation.Acquired {
		t.Fatal("reservation was not acquired")
	}
	if result.Diagnostics.Total != 1 {
		t.Fatalf("diagnostics total = %d, want 1", result.Diagnostics.Total)
	}
	if result.Diagnostics.Eligible != 1 {
		t.Fatalf("diagnostics eligible = %d, want 1", result.Diagnostics.Eligible)
	}
}

func TestOpenAISchedulerSessionStickyFallsBackWhenTransportMismatch(t *testing.T) {
	ports := newFakePorts()
	ports.sticky[stickyKey(1, "session_1")] = 10
	ports.accounts[10] = testAccount(10, string(domain.AccountTypeAPIKey), "gpt-5.1", domain.TransportHTTP)
	ports.accounts[20] = testAccount(20, string(domain.AccountTypeAPIKey), "gpt-5.1", domain.TransportWebSocket)

	result, err := NewOpenAIScheduler(ports).Select(context.Background(), ScheduleRequest{
		GroupID:           1,
		SessionKey:        "session_1",
		RequestedModel:    "gpt-5.1",
		RequiredTransport: domain.TransportWebSocket,
	})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}

	if result.Account.Snapshot.ID != 20 {
		t.Fatalf("account ID = %d, want 20", result.Account.Snapshot.ID)
	}
	if result.Layer != domain.AccountDecisionLoadBalance {
		t.Fatalf("layer = %q, want %q", result.Layer, domain.AccountDecisionLoadBalance)
	}
	if got := result.Diagnostics.RejectCount[domain.RejectionReasonTransportMismatch]; got != 1 {
		t.Fatalf("transport mismatch rejections = %d, want 1", got)
	}
	if result.Diagnostics.Total != 2 {
		t.Fatalf("diagnostics total = %d, want 2", result.Diagnostics.Total)
	}
	if result.Diagnostics.Eligible != 1 {
		t.Fatalf("diagnostics eligible = %d, want 1", result.Diagnostics.Eligible)
	}
	if result.Diagnostics.Rejected != 1 {
		t.Fatalf("diagnostics rejected = %d, want 1", result.Diagnostics.Rejected)
	}
	if got := ports.sticky[stickyKey(1, "session_1")]; got != 20 {
		t.Fatalf("sticky account = %d, want rebound to 20", got)
	}
	if !ports.deletedSticky[stickyKey(1, "session_1")] {
		t.Fatal("sticky session was not deleted before fallback")
	}
}

func TestOpenAISchedulerLoadBalanceExcludesFailedAccounts(t *testing.T) {
	ports := newFakePorts()
	ports.accounts[10] = testAccount(10, string(domain.AccountTypeAPIKey), "gpt-5.1")
	ports.accounts[20] = testAccount(20, string(domain.AccountTypeAPIKey), "gpt-5.1")

	result, err := NewOpenAIScheduler(ports).Select(context.Background(), ScheduleRequest{
		GroupID:            1,
		RequestedModel:     "gpt-5.1",
		ExcludedAccountIDs: map[int64]struct{}{10: {}},
	})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}

	if result.Account.Snapshot.ID != 20 {
		t.Fatalf("account ID = %d, want 20", result.Account.Snapshot.ID)
	}
	if got := result.Diagnostics.RejectCount[domain.RejectionReasonExcluded]; got != 1 {
		t.Fatalf("excluded rejections = %d, want 1", got)
	}
}

func TestOpenAISchedulerNoAccountReturnsDiagnostics(t *testing.T) {
	ports := newFakePorts()
	ports.accounts[10] = testAccount(10, string(domain.AccountTypeAPIKey), "gpt-4o")

	_, err := NewOpenAIScheduler(ports).Select(context.Background(), ScheduleRequest{
		GroupID:        1,
		RequestedModel: "gpt-5.1",
	})
	if err == nil {
		t.Fatal("Select() error = nil, want no available accounts error")
	}
	if !errors.Is(err, ErrNoAvailableAccounts) {
		t.Fatalf("errors.Is(err, ErrNoAvailableAccounts) = false for %v", err)
	}
	var noAvailable *NoAvailableAccountsError
	if !errors.As(err, &noAvailable) {
		t.Fatalf("errors.As(err, *NoAvailableAccountsError) = false for %T", err)
	}
	if got := noAvailable.Diagnostics.RejectCount[domain.RejectionReasonModelUnsupported]; got != 1 {
		t.Fatalf("model unsupported rejections = %d, want 1", got)
	}
}

func TestOpenAISchedulerWaitPlanWhenReservationBusy(t *testing.T) {
	ports := newFakePorts()
	ports.accounts[10] = testAccount(10, string(domain.AccountTypeAPIKey), "gpt-5.1")
	ports.busy[10] = true

	result, err := NewOpenAIScheduler(ports).Select(context.Background(), ScheduleRequest{
		GroupID:        1,
		RequestedModel: "gpt-5.1",
	})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}

	if result.Account.Snapshot.ID != 10 {
		t.Fatalf("account ID = %d, want 10", result.Account.Snapshot.ID)
	}
	if result.Reservation.Acquired {
		t.Fatal("reservation acquired, want busy")
	}
	if !result.WaitPlan.Required {
		t.Fatalf("wait plan required = false, want true")
	}
}

func TestOpenAISchedulerLoadBalanceContinuesPastBusyAccount(t *testing.T) {
	ports := newFakePorts()
	ports.accounts[10] = testAccount(10, string(domain.AccountTypeAPIKey), "gpt-5.1")
	ports.accounts[20] = testAccount(20, string(domain.AccountTypeAPIKey), "gpt-5.1")
	ports.busy[10] = true

	result, err := NewOpenAIScheduler(ports).Select(context.Background(), ScheduleRequest{
		GroupID:        1,
		RequestedModel: "gpt-5.1",
	})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}

	if result.Account.Snapshot.ID != 20 {
		t.Fatalf("account ID = %d, want 20", result.Account.Snapshot.ID)
	}
	if !result.Reservation.Acquired {
		t.Fatal("reservation was not acquired")
	}
	if result.WaitPlan.Required {
		t.Fatalf("wait plan required = true, want false")
	}
	if result.Diagnostics.Total != 2 {
		t.Fatalf("diagnostics total = %d, want 2", result.Diagnostics.Total)
	}
	if result.Diagnostics.Eligible != 2 {
		t.Fatalf("diagnostics eligible = %d, want 2", result.Diagnostics.Eligible)
	}
}

func TestOpenAISchedulerLoadBalanceRechecksListedAccountFreshness(t *testing.T) {
	ports := newFakePorts()
	ports.accounts[10] = testAccount(10, string(domain.AccountTypeAPIKey), "gpt-5.1")
	ports.accounts[20] = testAccount(20, string(domain.AccountTypeAPIKey), "gpt-5.1")
	ports.missingOnGet[10] = true

	result, err := NewOpenAIScheduler(ports).Select(context.Background(), ScheduleRequest{
		GroupID:        1,
		RequestedModel: "gpt-5.1",
	})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}

	if result.Account.Snapshot.ID != 20 {
		t.Fatalf("account ID = %d, want 20", result.Account.Snapshot.ID)
	}
	if got := result.Diagnostics.RejectCount[domain.RejectionReasonUnschedulable]; got != 1 {
		t.Fatalf("unschedulable rejections = %d, want 1", got)
	}
	if result.Diagnostics.Total != 2 {
		t.Fatalf("diagnostics total = %d, want 2", result.Diagnostics.Total)
	}
	if result.Diagnostics.Eligible != 1 {
		t.Fatalf("diagnostics eligible = %d, want 1", result.Diagnostics.Eligible)
	}
}

func TestOpenAISchedulerLoadBalanceSortsByPriorityThenID(t *testing.T) {
	ports := newFakePorts()
	account30 := testAccount(30, string(domain.AccountTypeAPIKey), "gpt-5.1")
	account10 := testAccount(10, string(domain.AccountTypeAPIKey), "gpt-5.1")
	account20 := testAccount(20, string(domain.AccountTypeAPIKey), "gpt-5.1")
	account30.Snapshot.Priority = 1
	account10.Snapshot.Priority = 2
	account20.Snapshot.Priority = 1
	ports.accounts[30] = account30
	ports.accounts[10] = account10
	ports.accounts[20] = account20

	result, err := NewOpenAIScheduler(ports).Select(context.Background(), ScheduleRequest{
		GroupID:        1,
		RequestedModel: "gpt-5.1",
	})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}

	if result.Account.Snapshot.ID != 20 {
		t.Fatalf("account ID = %d, want 20", result.Account.Snapshot.ID)
	}
}

func TestOpenAISchedulerDefaultHTTPTransport(t *testing.T) {
	ports := newFakePorts()
	ports.accounts[10] = testAccount(10, string(domain.AccountTypeAPIKey), "gpt-5.1")

	result, err := NewOpenAIScheduler(ports).Select(context.Background(), ScheduleRequest{
		GroupID:        1,
		RequestedModel: "gpt-5.1",
	})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}

	if result.Account.Snapshot.ID != 10 {
		t.Fatalf("account ID = %d, want 10", result.Account.Snapshot.ID)
	}
}

func TestOpenAISchedulerModelWildcardSupportsPrefixMatch(t *testing.T) {
	ports := newFakePorts()
	ports.accounts[10] = testAccount(10, string(domain.AccountTypeAPIKey), "claude-*")

	result, err := NewOpenAIScheduler(ports).Select(context.Background(), ScheduleRequest{
		GroupID:        1,
		RequestedModel: "claude-sonnet-4-5",
	})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}

	if result.Account.Snapshot.ID != 10 {
		t.Fatalf("account ID = %d, want 10", result.Account.Snapshot.ID)
	}
}

func TestOpenAISchedulerModelWildcardRejectsUnrelatedModel(t *testing.T) {
	ports := newFakePorts()
	ports.accounts[10] = testAccount(10, string(domain.AccountTypeAPIKey), "claude-*")
	ports.accounts[20] = testAccount(20, string(domain.AccountTypeAPIKey), "gemini-*")

	result, err := NewOpenAIScheduler(ports).Select(context.Background(), ScheduleRequest{
		GroupID:        1,
		RequestedModel: "gemini-3.1-pro-preview",
	})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}

	if result.Account.Snapshot.ID != 20 {
		t.Fatalf("account ID = %d, want 20", result.Account.Snapshot.ID)
	}
	if got := result.Diagnostics.RejectCount[domain.RejectionReasonModelUnsupported]; got != 1 {
		t.Fatalf("model unsupported rejections = %d, want 1", got)
	}
}

func TestOpenAISchedulerExactModelMatchIsCaseSensitive(t *testing.T) {
	ports := newFakePorts()
	ports.accounts[10] = testAccount(10, string(domain.AccountTypeAPIKey), "GPT-4o")

	_, err := NewOpenAIScheduler(ports).Select(context.Background(), ScheduleRequest{
		GroupID:        1,
		RequestedModel: "gpt-4o",
	})
	if err == nil {
		t.Fatal("Select() error = nil, want no available accounts error")
	}
	if !errors.Is(err, ErrNoAvailableAccounts) {
		t.Fatalf("errors.Is(err, ErrNoAvailableAccounts) = false for %v", err)
	}
	var noAvailable *NoAvailableAccountsError
	if !errors.As(err, &noAvailable) {
		t.Fatalf("errors.As(err, *NoAvailableAccountsError) = false for %T", err)
	}
	if got := noAvailable.Diagnostics.RejectCount[domain.RejectionReasonModelUnsupported]; got != 1 {
		t.Fatalf("model unsupported rejections = %d, want 1", got)
	}
}

func TestOpenAISchedulerModelWildcardStarMatchesAnyModel(t *testing.T) {
	if !supportsModel(domain.PlatformOpenAI, []string{"*"}, "any-model") {
		t.Fatal("star wildcard did not match arbitrary model")
	}
}

func TestOpenAISchedulerModelWildcardIsCaseSensitive(t *testing.T) {
	if supportsModel(domain.PlatformOpenAI, []string{"CLAUDE-*"}, "claude-sonnet-4-5") {
		t.Fatal("wildcard matched with different case, want legacy case-sensitive behavior")
	}
}

func TestOpenAISchedulerGeminiCustomToolsAliasUsesNormalizedModel(t *testing.T) {
	if !supportsModel(domain.PlatformGemini, []string{"gemini-3.1-pro-preview"}, "gemini-3.1-pro-preview-customtools") {
		t.Fatal("Gemini customtools alias was not normalized")
	}
}

func TestOpenAISchedulerAntigravityCustomToolsAliasUsesNormalizedModel(t *testing.T) {
	if !supportsModel(domain.PlatformAntigravity, []string{"gemini-3.1-pro-preview"}, "gemini-3.1-pro-preview-customtools") {
		t.Fatal("Antigravity customtools alias was not normalized")
	}
}

type fakePorts struct {
	accounts      map[int64]Account
	previous      map[string]int64
	sticky        map[string]int64
	deletedSticky map[string]bool
	busy          map[int64]bool
	missingOnGet  map[int64]bool
}

func newFakePorts() *fakePorts {
	return &fakePorts{
		accounts:      make(map[int64]Account),
		previous:      make(map[string]int64),
		sticky:        make(map[string]int64),
		deletedSticky: make(map[string]bool),
		busy:          make(map[int64]bool),
		missingOnGet:  make(map[int64]bool),
	}
}

func testAccount(id int64, accountType string, model string, transports ...domain.TransportKind) Account {
	models := []string{}
	if model != "" {
		models = []string{model}
	}
	return Account{
		Snapshot: domain.AccountSnapshot{
			ID:          id,
			Platform:    domain.PlatformOpenAI,
			Type:        domain.AccountType(accountType),
			Priority:    int(id),
			Concurrency: 1,
			Capabilities: domain.AccountCapabilities{
				Models:     models,
				Transports: transports,
				Streaming:  true,
			},
		},
	}
}

func stickyKey(groupID int64, sessionKey string) string {
	return fmt.Sprintf("group:%d:%s", groupID, sessionKey)
}

func (f *fakePorts) LookupPreviousResponseAccount(_ context.Context, _ int64, previousResponseID string) (int64, bool, error) {
	accountID, ok := f.previous[previousResponseID]
	return accountID, ok, nil
}

func (f *fakePorts) GetStickySessionAccount(_ context.Context, groupID int64, sessionKey string) (int64, bool, error) {
	accountID, ok := f.sticky[stickyKey(groupID, sessionKey)]
	return accountID, ok, nil
}

func (f *fakePorts) BindStickySession(_ context.Context, groupID int64, sessionKey string, accountID int64) error {
	f.sticky[stickyKey(groupID, sessionKey)] = accountID
	return nil
}

func (f *fakePorts) DeleteStickySession(_ context.Context, groupID int64, sessionKey string) error {
	key := stickyKey(groupID, sessionKey)
	delete(f.sticky, key)
	f.deletedSticky[key] = true
	return nil
}

func (f *fakePorts) RefreshStickySession(_ context.Context, _ int64, _ string) error {
	return nil
}

func (f *fakePorts) ListSchedulableOpenAIAccounts(_ context.Context, _ int64) ([]Account, error) {
	accounts := make([]Account, 0, len(f.accounts))
	for _, account := range f.accounts {
		accounts = append(accounts, account)
	}
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Snapshot.ID < accounts[j].Snapshot.ID
	})
	return accounts, nil
}

func (f *fakePorts) GetAccount(_ context.Context, accountID int64) (Account, bool, error) {
	if f.missingOnGet[accountID] {
		return Account{}, false, nil
	}
	account, ok := f.accounts[accountID]
	return account, ok, nil
}

func (f *fakePorts) AcquireAccountSlot(_ context.Context, account Account) (Reservation, error) {
	reservation := Reservation{AccountID: account.Snapshot.ID}
	if f.busy[account.Snapshot.ID] {
		return reservation, nil
	}
	reservation.Acquired = true
	reservation.Release = func() {}
	return reservation, nil
}

func (f *fakePorts) WaitPlan(_ context.Context, account Account) domain.AccountWaitPlan {
	return domain.AccountWaitPlan{
		Required: true,
		Reason:   "account_busy",
		Timeout:  time.Second,
	}
}

func (f *fakePorts) ReportResult(_ context.Context, _ int64, _ ScheduleOutcome) {}
