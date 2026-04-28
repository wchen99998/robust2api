package adapters

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	"github.com/Wei-Shaw/sub2api/internal/gateway/scheduler"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestOpenAIAccountToSchedulerAccountMapsSnapshotAndCapabilities(t *testing.T) {
	legacy := &service.Account{
		ID:          42,
		Name:        "oauth-ws",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Priority:    2,
		Concurrency: 3,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"gpt-5.1": "gpt-5.1"},
		},
	}

	account := OpenAIAccountToSchedulerAccount(legacy, []domain.TransportKind{
		domain.TransportHTTP,
		domain.TransportWebSocket,
	})

	if account.Snapshot.ID != 42 {
		t.Fatalf("ID = %d, want 42", account.Snapshot.ID)
	}
	if account.Snapshot.Name != "oauth-ws" {
		t.Fatalf("Name = %q, want oauth-ws", account.Snapshot.Name)
	}
	if account.Snapshot.Platform != domain.PlatformOpenAI {
		t.Fatalf("Platform = %q, want %q", account.Snapshot.Platform, domain.PlatformOpenAI)
	}
	if account.Snapshot.Type != domain.AccountTypeOAuth {
		t.Fatalf("Type = %q, want %q", account.Snapshot.Type, domain.AccountTypeOAuth)
	}
	if account.Snapshot.Priority != 2 {
		t.Fatalf("Priority = %d, want 2", account.Snapshot.Priority)
	}
	if account.Snapshot.Concurrency != 3 {
		t.Fatalf("Concurrency = %d, want 3", account.Snapshot.Concurrency)
	}
	if !reflect.DeepEqual(account.Snapshot.Capabilities.Models, []string{"gpt-5.1"}) {
		t.Fatalf("Models = %#v, want [gpt-5.1]", account.Snapshot.Capabilities.Models)
	}
	if !reflect.DeepEqual(account.Snapshot.Capabilities.Transports, []domain.TransportKind{domain.TransportHTTP, domain.TransportWebSocket}) {
		t.Fatalf("Transports = %#v, want HTTP and WebSocket", account.Snapshot.Capabilities.Transports)
	}
	if !account.Snapshot.Capabilities.Streaming {
		t.Fatal("Streaming = false, want true")
	}
	if account.Legacy != legacy {
		t.Fatalf("Legacy pointer = %#v, want original account", account.Legacy)
	}
}

func TestOpenAIAccountToSchedulerAccountNilReturnsZero(t *testing.T) {
	if got := OpenAIAccountToSchedulerAccount(nil, []domain.TransportKind{domain.TransportHTTP}); !reflect.DeepEqual(got, scheduler.Account{}) {
		t.Fatalf("nil account maps to %#v, want zero scheduler.Account", got)
	}
}

func TestOpenAIAccountToSchedulerAccountExposesWildcardModelMappingKeys(t *testing.T) {
	legacy := &service.Account{
		ID:       42,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"claude-*": "claude-sonnet-4-5"},
		},
	}

	account := OpenAIAccountToSchedulerAccount(legacy, []domain.TransportKind{domain.TransportHTTP})

	if !reflect.DeepEqual(account.Snapshot.Capabilities.Models, []string{"claude-*"}) {
		t.Fatalf("Models = %#v, want [claude-*]", account.Snapshot.Capabilities.Models)
	}
}

func TestOpenAISchedulerPortsWaitPlanDelegatesAndMapsBridgeBehavior(t *testing.T) {
	ctx := context.Background()
	groupID := int64(7)
	releaseCalled := false
	bridge := &fakeOpenAISchedulerBridge{
		previousAccountID: 42,
		accounts: []service.Account{
			{
				ID:       42,
				Name:     "oauth-ws",
				Platform: service.PlatformOpenAI,
				Type:     service.AccountTypeOAuth,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"gpt-5.1": "gpt-5.1"},
				},
			},
		},
		accountByID: map[int64]*service.Account{
			42: {
				ID:          42,
				Name:        "oauth-ws",
				Platform:    service.PlatformOpenAI,
				Type:        service.AccountTypeOAuth,
				Concurrency: 3,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"gpt-5.1": "gpt-5.1"},
				},
			},
		},
		transportsByID: map[int64][]domain.TransportKind{
			42: {domain.TransportHTTP, domain.TransportWebSocket},
		},
		acquireResult: &service.AcquireResult{
			Acquired: true,
			ReleaseFunc: func() {
				releaseCalled = true
			},
		},
		waitPlan: domain.AccountWaitPlan{
			Required:       true,
			Reason:         "account_busy",
			Timeout:        12 * time.Second,
			MaxConcurrency: 3,
			MaxWaiting:     17,
		},
	}
	excluded := map[int64]struct{}{99: {}}
	ports := OpenAISchedulerPorts{
		Bridge:         bridge,
		GroupID:        &groupID,
		RequestedModel: "gpt-5.1",
		Excluded:       excluded,
	}

	accountID, ok, err := ports.LookupPreviousResponseAccount(ctx, 123, "resp_1")
	if err != nil {
		t.Fatalf("LookupPreviousResponseAccount() error = %v", err)
	}
	if !ok || accountID != 42 {
		t.Fatalf("previous lookup = (%d, %v), want (42, true)", accountID, ok)
	}
	if bridge.previousGroupID == nil || *bridge.previousGroupID != groupID {
		t.Fatalf("previous group ID = %#v, want %d", bridge.previousGroupID, groupID)
	}
	if bridge.previousRequestedModel != "gpt-5.1" {
		t.Fatalf("previous requested model = %q, want gpt-5.1", bridge.previousRequestedModel)
	}
	if !reflect.DeepEqual(bridge.previousExcluded, excluded) {
		t.Fatalf("previous excluded = %#v, want %#v", bridge.previousExcluded, excluded)
	}

	accounts, err := ports.ListSchedulableOpenAIAccounts(ctx, 123)
	if err != nil {
		t.Fatalf("ListSchedulableOpenAIAccounts() error = %v", err)
	}
	if len(accounts) != 1 || accounts[0].Snapshot.ID != 42 {
		t.Fatalf("accounts = %#v, want account 42", accounts)
	}
	if !reflect.DeepEqual(accounts[0].Snapshot.Capabilities.Transports, []domain.TransportKind{domain.TransportHTTP, domain.TransportWebSocket}) {
		t.Fatalf("mapped transports = %#v, want HTTP and WebSocket", accounts[0].Snapshot.Capabilities.Transports)
	}

	account, ok, err := ports.GetAccount(ctx, 42)
	if err != nil {
		t.Fatalf("GetAccount() error = %v", err)
	}
	if !ok || account.Snapshot.ID != 42 {
		t.Fatalf("GetAccount() = (%#v, %v), want account 42", account, ok)
	}
	if bridge.getRequestedModel != "gpt-5.1" {
		t.Fatalf("GetAccount requested model = %q, want gpt-5.1", bridge.getRequestedModel)
	}

	reservation, err := ports.AcquireAccountSlot(ctx, account)
	if err != nil {
		t.Fatalf("AcquireAccountSlot() error = %v", err)
	}
	if reservation.AccountID != 42 || !reservation.Acquired {
		t.Fatalf("reservation = %#v, want acquired account 42", reservation)
	}
	if bridge.acquireAccountID != 42 || bridge.acquireMaxConcurrency != 3 {
		t.Fatalf("acquire args = (%d, %d), want (42, 3)", bridge.acquireAccountID, bridge.acquireMaxConcurrency)
	}
	reservation.Release()
	if !releaseCalled {
		t.Fatal("reservation release did not delegate")
	}

	waitPlan := ports.WaitPlan(ctx, account, domain.AccountDecisionSessionHash)
	if waitPlan.Reason != "account_busy" || waitPlan.Timeout != 12*time.Second || waitPlan.MaxConcurrency != 3 || waitPlan.MaxWaiting != 17 {
		t.Fatalf("wait plan = %#v, want account_busy/12s/max concurrency 3/max waiting 17", waitPlan)
	}
	if bridge.waitPlanLayer != domain.AccountDecisionSessionHash {
		t.Fatalf("wait plan layer = %q, want %q", bridge.waitPlanLayer, domain.AccountDecisionSessionHash)
	}

	firstTokenMs := 321
	ports.ReportResult(ctx, 42, scheduler.ScheduleOutcome{Success: true, FirstTokenMs: &firstTokenMs})
	if bridge.reportAccountID != 42 || !bridge.reportSuccess || bridge.reportFirstTokenMs != &firstTokenMs {
		t.Fatalf("report args = (%d, %v, %#v), want (42, true, firstTokenMs)", bridge.reportAccountID, bridge.reportSuccess, bridge.reportFirstTokenMs)
	}
}

type fakeOpenAISchedulerBridge struct {
	previousAccountID      int64
	previousGroupID        *int64
	previousRequestedModel string
	previousExcluded       map[int64]struct{}

	accounts       []service.Account
	accountByID    map[int64]*service.Account
	transportsByID map[int64][]domain.TransportKind
	acquireResult  *service.AcquireResult
	waitPlan       domain.AccountWaitPlan
	waitPlanLayer  domain.AccountDecisionLayer

	acquireAccountID      int64
	acquireMaxConcurrency int
	reportAccountID       int64
	reportSuccess         bool
	reportFirstTokenMs    *int
	getRequestedModel     string
}

func (f *fakeOpenAISchedulerBridge) GatewayLookupPreviousResponseAccount(_ context.Context, groupID *int64, _ string, requestedModel string, excluded map[int64]struct{}) (int64, bool, error) {
	f.previousGroupID = groupID
	f.previousRequestedModel = requestedModel
	f.previousExcluded = excluded
	return f.previousAccountID, f.previousAccountID > 0, nil
}

func (f *fakeOpenAISchedulerBridge) GatewayGetStickySessionAccountID(context.Context, *int64, string) (int64, bool, error) {
	return 0, false, nil
}

func (f *fakeOpenAISchedulerBridge) BindStickySession(context.Context, *int64, string, int64) error {
	return nil
}

func (f *fakeOpenAISchedulerBridge) GatewayDeleteStickySessionAccountID(context.Context, *int64, string) error {
	return nil
}

func (f *fakeOpenAISchedulerBridge) GatewayRefreshStickySessionTTL(context.Context, *int64, string) error {
	return nil
}

func (f *fakeOpenAISchedulerBridge) GatewayListSchedulableOpenAIAccounts(context.Context, *int64) ([]service.Account, error) {
	return f.accounts, nil
}

func (f *fakeOpenAISchedulerBridge) GatewayGetSchedulableOpenAIAccount(_ context.Context, accountID int64, requestedModel string) (*service.Account, error) {
	f.getRequestedModel = requestedModel
	return f.accountByID[accountID], nil
}

func (f *fakeOpenAISchedulerBridge) GatewayResolveOpenAITransports(account *service.Account) []domain.TransportKind {
	return f.transportsByID[account.ID]
}

func (f *fakeOpenAISchedulerBridge) GatewayAcquireAccountSlot(_ context.Context, accountID int64, maxConcurrency int) (*service.AcquireResult, error) {
	f.acquireAccountID = accountID
	f.acquireMaxConcurrency = maxConcurrency
	return f.acquireResult, nil
}

func (f *fakeOpenAISchedulerBridge) GatewayAccountWaitPlan(_ context.Context, _ *service.Account, layer domain.AccountDecisionLayer) domain.AccountWaitPlan {
	f.waitPlanLayer = layer
	return f.waitPlan
}

func (f *fakeOpenAISchedulerBridge) ReportOpenAIAccountScheduleResult(accountID int64, success bool, firstTokenMs *int) {
	f.reportAccountID = accountID
	f.reportSuccess = success
	f.reportFirstTokenMs = firstTokenMs
}
