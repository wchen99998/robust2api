package scheduler

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type Selector struct{}

func NewSelector() *Selector {
	return &Selector{}
}

type SelectInput struct {
	Plan       core.RoutingPlan
	Accounts   []*service.Account
	ExcludedID map[int64]struct{}
}

func (s *Selector) Select(ctx context.Context, plan core.RoutingPlan, accounts []*service.Account, excluded map[int64]struct{}) core.AccountSelection {
	_ = ctx
	diagnostics := NewDiagnostics(len(accounts))
	for _, account := range accounts {
		if account == nil {
			diagnostics.Reject(RejectUnschedulable)
			continue
		}
		if _, isExcluded := excluded[account.ID]; isExcluded {
			diagnostics.Reject(RejectExcluded)
			continue
		}
		if forced := forcedPlatform(plan); forced != "" && account.Platform != forced {
			diagnostics.Reject(RejectForcePlatformMismatch)
			continue
		}
		if expected := strings.TrimSpace(plan.Provider); expected != "" && account.Platform != expected {
			diagnostics.Reject(RejectPlatformMismatch)
			continue
		}
		if account.IsRateLimited() {
			diagnostics.Reject(RejectRPMLimited)
			continue
		}
		if !account.IsSchedulable() {
			diagnostics.Reject(RejectUnschedulable)
			continue
		}
		if inputModel := strings.TrimSpace(plan.Model.UpstreamModel); inputModel != "" && !account.IsModelSupported(inputModel) {
			diagnostics.Reject(RejectModelUnsupported)
			continue
		}
		if account.IsQuotaExceeded() {
			diagnostics.Reject(RejectQuotaExhausted)
			continue
		}
		diagnostics.Accept()
		return core.AccountSelection{
			Account: account,
			Decision: core.AccountDecision{
				AccountID: account.ID,
				Platform:  account.Platform,
				Type:      account.Type,
				Acquired:  false,
			},
			Diagnostics: diagnostics.Snapshot(),
		}
	}
	return core.AccountSelection{Diagnostics: diagnostics.Snapshot()}
}

func (s *Selector) SelectInput(ctx context.Context, input SelectInput) core.AccountSelection {
	return s.Select(ctx, input.Plan, input.Accounts, input.ExcludedID)
}

func forcedPlatform(plan core.RoutingPlan) string {
	if len(plan.Meta) == 0 {
		return ""
	}
	if forced, _ := plan.Meta["force_platform"].(string); strings.TrimSpace(forced) != "" {
		return strings.TrimSpace(forced)
	}
	return ""
}
