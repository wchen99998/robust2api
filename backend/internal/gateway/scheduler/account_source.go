package scheduler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type AccountSource struct {
	repo service.AccountRepository
}

func NewAccountSource(repo service.AccountRepository) *AccountSource {
	return &AccountSource{repo: repo}
}

func (s *AccountSource) ListAccounts(ctx context.Context, plan core.RoutingPlan) ([]*service.Account, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	var accounts []service.Account
	var err error
	if plan.GroupID != nil && plan.Provider != "" {
		accounts, err = s.repo.ListSchedulableByGroupIDAndPlatform(ctx, *plan.GroupID, plan.Provider)
	} else if plan.GroupID != nil {
		accounts, err = s.repo.ListSchedulableByGroupID(ctx, *plan.GroupID)
	} else if plan.Provider != "" {
		accounts, err = s.repo.ListSchedulableByPlatform(ctx, plan.Provider)
	} else {
		accounts, err = s.repo.ListSchedulable(ctx)
	}
	if err != nil {
		return nil, err
	}
	out := make([]*service.Account, 0, len(accounts))
	for i := range accounts {
		account := accounts[i]
		out = append(out, &account)
	}
	return out, nil
}
