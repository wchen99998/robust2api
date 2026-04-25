package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T {
	return &v
}

func assertJSONTokenOrder(t *testing.T, body string, tokens ...string) {
	t.Helper()
	last := -1
	for _, token := range tokens {
		idx := strings.Index(body, token)
		require.NotEqual(t, -1, idx, "token %s missing from %s", token, body)
		require.Greater(t, idx, last, "token %s is out of order in %s", token, body)
		last = idx
	}
}

type mockAccountRepoForGemini struct {
	accounts           []Account
	accountsByID       map[int64]*Account
	listByGroupFunc    func(ctx context.Context, groupID int64, platforms []string) ([]Account, error)
	listByPlatformFunc func(ctx context.Context, platforms []string) ([]Account, error)
}

var _ AccountRepository = (*mockAccountRepoForGemini)(nil)

func (m *mockAccountRepoForGemini) GetByID(_ context.Context, id int64) (*Account, error) {
	if acc, ok := m.accountsByID[id]; ok {
		return acc, nil
	}
	for i := range m.accounts {
		if m.accounts[i].ID == id {
			return &m.accounts[i], nil
		}
	}
	return nil, errors.New("account not found")
}

func (m *mockAccountRepoForGemini) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	var result []*Account
	for _, id := range ids {
		if acc, ok := m.accountsByID[id]; ok {
			result = append(result, acc)
			continue
		}
		for i := range m.accounts {
			if m.accounts[i].ID == id {
				result = append(result, &m.accounts[i])
			}
		}
	}
	return result, nil
}

func (m *mockAccountRepoForGemini) ExistsByID(_ context.Context, id int64) (bool, error) {
	if m.accountsByID != nil {
		_, ok := m.accountsByID[id]
		return ok, nil
	}
	for _, acc := range m.accounts {
		if acc.ID == id {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockAccountRepoForGemini) ListSchedulableByPlatform(_ context.Context, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range m.accounts {
		if acc.Platform == platform && acc.IsSchedulable() {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (m *mockAccountRepoForGemini) ListSchedulableByGroupIDAndPlatform(ctx context.Context, _ int64, platform string) ([]Account, error) {
	return m.ListSchedulableByPlatform(ctx, platform)
}

func (m *mockAccountRepoForGemini) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	if m.listByPlatformFunc != nil {
		return m.listByPlatformFunc(ctx, platforms)
	}
	platformSet := make(map[string]bool, len(platforms))
	for _, p := range platforms {
		platformSet[p] = true
	}
	var result []Account
	for _, acc := range m.accounts {
		if platformSet[acc.Platform] && acc.IsSchedulable() {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (m *mockAccountRepoForGemini) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]Account, error) {
	if m.listByGroupFunc != nil {
		return m.listByGroupFunc(ctx, groupID, platforms)
	}
	return m.ListSchedulableByPlatforms(ctx, platforms)
}

func (m *mockAccountRepoForGemini) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return m.ListSchedulableByPlatform(ctx, platform)
}

func (m *mockAccountRepoForGemini) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	return m.ListSchedulableByPlatforms(ctx, platforms)
}

func (m *mockAccountRepoForGemini) Create(context.Context, *Account) error { return nil }
func (m *mockAccountRepoForGemini) GetByCRSAccountID(context.Context, string) (*Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) FindByExtraField(context.Context, string, any) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) ListCRSAccountIDs(context.Context) (map[string]int64, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) Update(context.Context, *Account) error { return nil }
func (m *mockAccountRepoForGemini) Delete(context.Context, int64) error    { return nil }
func (m *mockAccountRepoForGemini) List(context.Context, pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockAccountRepoForGemini) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, string, int64, string) ([]Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockAccountRepoForGemini) ListByGroup(context.Context, int64) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) ListActive(context.Context) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) ListByPlatform(context.Context, string) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) UpdateLastUsed(context.Context, int64) error { return nil }
func (m *mockAccountRepoForGemini) BatchUpdateLastUsed(context.Context, map[int64]time.Time) error {
	return nil
}
func (m *mockAccountRepoForGemini) SetError(context.Context, int64, string) error { return nil }
func (m *mockAccountRepoForGemini) ClearError(context.Context, int64) error       { return nil }
func (m *mockAccountRepoForGemini) SetSchedulable(context.Context, int64, bool) error {
	return nil
}
func (m *mockAccountRepoForGemini) AutoPauseExpiredAccounts(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (m *mockAccountRepoForGemini) BindGroups(context.Context, int64, []int64) error {
	return nil
}
func (m *mockAccountRepoForGemini) ListSchedulable(context.Context) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) SetRateLimited(context.Context, int64, time.Time) error {
	return nil
}
func (m *mockAccountRepoForGemini) SetModelRateLimit(context.Context, int64, string, time.Time) error {
	return nil
}
func (m *mockAccountRepoForGemini) SetOverloaded(context.Context, int64, time.Time) error {
	return nil
}
func (m *mockAccountRepoForGemini) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	return nil
}
func (m *mockAccountRepoForGemini) ClearTempUnschedulable(context.Context, int64) error {
	return nil
}
func (m *mockAccountRepoForGemini) ClearRateLimit(context.Context, int64) error { return nil }
func (m *mockAccountRepoForGemini) ClearAntigravityQuotaScopes(context.Context, int64) error {
	return nil
}
func (m *mockAccountRepoForGemini) ClearModelRateLimits(context.Context, int64) error {
	return nil
}
func (m *mockAccountRepoForGemini) UpdateSessionWindow(context.Context, int64, *time.Time, *time.Time, string) error {
	return nil
}
func (m *mockAccountRepoForGemini) UpdateExtra(context.Context, int64, map[string]any) error {
	return nil
}
func (m *mockAccountRepoForGemini) BulkUpdate(context.Context, []int64, AccountBulkUpdate) (int64, error) {
	return 0, nil
}
func (m *mockAccountRepoForGemini) IncrementQuotaUsed(context.Context, int64, float64) error {
	return nil
}
func (m *mockAccountRepoForGemini) ResetQuotaUsed(context.Context, int64) error { return nil }

type stubOpenAIAccountRepo struct {
	AccountRepository
	accounts []Account
}

var _ AccountRepository = (*stubOpenAIAccountRepo)(nil)

func (r *stubOpenAIAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			return &r.accounts[i], nil
		}
	}
	return nil, errors.New("account not found")
}

func (r *stubOpenAIAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, _ int64, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range r.accounts {
		if acc.Platform == platform {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (r *stubOpenAIAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return r.ListSchedulableByGroupIDAndPlatform(ctx, 0, platform)
}

func (r *stubOpenAIAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}
