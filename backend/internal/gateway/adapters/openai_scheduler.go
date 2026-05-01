package adapters

import (
	"context"
	"sort"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
	"github.com/Wei-Shaw/sub2api/internal/gateway/scheduler"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type OpenAISchedulerBridge interface {
	GatewayLookupPreviousResponseAccount(ctx context.Context, groupID *int64, previousResponseID string, requestedModel string, excluded map[int64]struct{}) (int64, bool, error)
	GatewayGetStickySessionAccountID(ctx context.Context, groupID *int64, sessionHash string) (int64, bool, error)
	BindStickySession(ctx context.Context, groupID *int64, sessionHash string, accountID int64) error
	GatewayDeleteStickySessionAccountID(ctx context.Context, groupID *int64, sessionHash string) error
	GatewayRefreshStickySessionTTL(ctx context.Context, groupID *int64, sessionHash string) error
	GatewayListSchedulableOpenAIAccounts(ctx context.Context, groupID *int64) ([]service.Account, error)
	GatewayGetSchedulableOpenAIAccount(ctx context.Context, accountID int64, requestedModel string) (*service.Account, error)
	GatewayResolveOpenAITransports(account *service.Account) []domain.TransportKind
	GatewayAcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int) (*service.AcquireResult, error)
	GatewayAccountWaitPlan(ctx context.Context, account *service.Account, layer domain.AccountDecisionLayer) domain.AccountWaitPlan
	ReportOpenAIAccountScheduleResult(accountID int64, success bool, firstTokenMs *int)
}

type OpenAISchedulerPorts struct {
	Bridge         OpenAISchedulerBridge
	GroupID        *int64
	RequestedModel string
	Excluded       map[int64]struct{}
}

var _ scheduler.Ports = OpenAISchedulerPorts{}

func OpenAIAccountToSchedulerAccount(account *service.Account, transports []domain.TransportKind) scheduler.Account {
	if account == nil {
		return scheduler.Account{}
	}
	return scheduler.Account{
		Snapshot: domain.AccountSnapshot{
			ID:          account.ID,
			Name:        account.Name,
			Platform:    mapOpenAIAccountPlatform(account.Platform),
			Type:        mapOpenAIAccountType(account.Type),
			Priority:    account.Priority,
			Concurrency: account.Concurrency,
			Capabilities: domain.AccountCapabilities{
				Models:     openAIAccountModels(account),
				Transports: append([]domain.TransportKind(nil), transports...),
				Streaming:  true,
			},
		},
		Legacy: account,
	}
}

func (p OpenAISchedulerPorts) LookupPreviousResponseAccount(ctx context.Context, groupID int64, previousResponseID string) (int64, bool, error) {
	if p.Bridge == nil {
		return 0, false, nil
	}
	return p.Bridge.GatewayLookupPreviousResponseAccount(ctx, p.bridgeGroupID(groupID), previousResponseID, p.RequestedModel, p.Excluded)
}

func (p OpenAISchedulerPorts) GetStickySessionAccount(ctx context.Context, groupID int64, sessionKey string) (int64, bool, error) {
	if p.Bridge == nil {
		return 0, false, nil
	}
	return p.Bridge.GatewayGetStickySessionAccountID(ctx, p.bridgeGroupID(groupID), sessionKey)
}

func (p OpenAISchedulerPorts) BindStickySession(ctx context.Context, groupID int64, sessionKey string, accountID int64) error {
	if p.Bridge == nil {
		return nil
	}
	return p.Bridge.BindStickySession(ctx, p.bridgeGroupID(groupID), sessionKey, accountID)
}

func (p OpenAISchedulerPorts) DeleteStickySession(ctx context.Context, groupID int64, sessionKey string) error {
	if p.Bridge == nil {
		return nil
	}
	return p.Bridge.GatewayDeleteStickySessionAccountID(ctx, p.bridgeGroupID(groupID), sessionKey)
}

func (p OpenAISchedulerPorts) RefreshStickySession(ctx context.Context, groupID int64, sessionKey string) error {
	if p.Bridge == nil {
		return nil
	}
	return p.Bridge.GatewayRefreshStickySessionTTL(ctx, p.bridgeGroupID(groupID), sessionKey)
}

func (p OpenAISchedulerPorts) ListSchedulableOpenAIAccounts(ctx context.Context, groupID int64) ([]scheduler.Account, error) {
	if p.Bridge == nil {
		return nil, nil
	}
	legacyAccounts, err := p.Bridge.GatewayListSchedulableOpenAIAccounts(ctx, p.bridgeGroupID(groupID))
	if err != nil {
		return nil, err
	}
	accounts := make([]scheduler.Account, 0, len(legacyAccounts))
	for i := range legacyAccounts {
		account := &legacyAccounts[i]
		accounts = append(accounts, OpenAIAccountToSchedulerAccount(account, p.Bridge.GatewayResolveOpenAITransports(account)))
	}
	return accounts, nil
}

func (p OpenAISchedulerPorts) GetAccount(ctx context.Context, accountID int64) (scheduler.Account, bool, error) {
	if p.Bridge == nil {
		return scheduler.Account{}, false, nil
	}
	account, err := p.Bridge.GatewayGetSchedulableOpenAIAccount(ctx, accountID, p.RequestedModel)
	if err != nil {
		return scheduler.Account{}, false, err
	}
	if account == nil {
		return scheduler.Account{}, false, nil
	}
	return OpenAIAccountToSchedulerAccount(account, p.Bridge.GatewayResolveOpenAITransports(account)), true, nil
}

func (p OpenAISchedulerPorts) AcquireAccountSlot(ctx context.Context, account scheduler.Account) (scheduler.Reservation, error) {
	accountID := account.Snapshot.ID
	if p.Bridge == nil {
		return scheduler.Reservation{AccountID: accountID}, nil
	}
	result, err := p.Bridge.GatewayAcquireAccountSlot(ctx, accountID, account.Snapshot.Concurrency)
	if err != nil {
		return scheduler.Reservation{}, err
	}
	if result == nil {
		return scheduler.Reservation{AccountID: accountID}, nil
	}
	return scheduler.Reservation{
		AccountID: accountID,
		Acquired:  result.Acquired,
		Release:   result.ReleaseFunc,
	}, nil
}

func (p OpenAISchedulerPorts) WaitPlan(ctx context.Context, account scheduler.Account, layer domain.AccountDecisionLayer) domain.AccountWaitPlan {
	if p.Bridge == nil {
		return domain.AccountWaitPlan{}
	}
	legacy, _ := account.Legacy.(*service.Account)
	waitPlan := p.Bridge.GatewayAccountWaitPlan(ctx, legacy, layer)
	if waitPlan.Required && waitPlan.Reason == "" {
		waitPlan.Reason = "account_busy"
	}
	return waitPlan
}

func (p OpenAISchedulerPorts) ReportResult(_ context.Context, accountID int64, outcome scheduler.ScheduleOutcome) {
	if p.Bridge == nil {
		return
	}
	p.Bridge.ReportOpenAIAccountScheduleResult(accountID, outcome.Success, outcome.FirstTokenMs)
}

func (p OpenAISchedulerPorts) bridgeGroupID(groupID int64) *int64 {
	if p.GroupID != nil {
		return p.GroupID
	}
	if groupID == 0 {
		return nil
	}
	return &groupID
}

func openAIAccountModels(account *service.Account) []string {
	if account == nil {
		return nil
	}
	mapping := account.GetModelMapping()
	if len(mapping) == 0 {
		return nil
	}
	models := make([]string, 0, len(mapping))
	for model := range mapping {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}

func mapOpenAIAccountPlatform(platform string) domain.Platform {
	switch platform {
	case service.PlatformOpenAI:
		return domain.PlatformOpenAI
	case service.PlatformAnthropic:
		return domain.PlatformAnthropic
	case service.PlatformGemini:
		return domain.PlatformGemini
	case service.PlatformAntigravity:
		return domain.PlatformAntigravity
	default:
		return domain.Platform(platform)
	}
}

func mapOpenAIAccountType(accountType string) domain.AccountType {
	switch accountType {
	case service.AccountTypeOAuth:
		return domain.AccountTypeOAuth
	case service.AccountTypeAPIKey:
		return domain.AccountTypeAPIKey
	default:
		return domain.AccountType(accountType)
	}
}
