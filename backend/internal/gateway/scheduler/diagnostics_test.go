package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestDiagnosticsSnapshotCopiesRejections(t *testing.T) {
	d := NewDiagnostics(3)
	d.Reject(RejectModelUnsupported)
	d.Reject(RejectModelUnsupported)
	d.Reject(RejectConcurrencyFull)
	d.Accept()

	snapshot := d.Snapshot()
	snapshot.RejectedBy[RejectModelUnsupported] = 99

	require.Equal(t, 3, snapshot.Total)
	require.Equal(t, 1, snapshot.Eligible)
	require.Equal(t, 2, d.Snapshot().RejectedBy[RejectModelUnsupported])
}

func TestSelectorSelectsFirstEligibleAccount(t *testing.T) {
	out := NewSelector().SelectInput(context.Background(), SelectInput{
		Plan: core.RoutingPlan{Provider: service.PlatformOpenAI, Model: core.ModelResolution{UpstreamModel: "gpt-5"}},
		Accounts: []*service.Account{
			{ID: 1, Platform: service.PlatformAnthropic, Status: service.StatusActive, Schedulable: true},
			{ID: 2, Platform: service.PlatformOpenAI, Status: service.StatusActive, Schedulable: true, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5": "gpt-5"}}},
		},
	})

	require.Equal(t, int64(2), out.Account.ID)
	require.Equal(t, int64(2), out.Decision.AccountID)
	require.Equal(t, 1, out.Diagnostics.Eligible)
	require.Equal(t, 1, out.Diagnostics.RejectedBy[RejectPlatformMismatch])
}

func TestSelectorReportsModelUnsupported(t *testing.T) {
	out := NewSelector().Select(context.Background(), core.RoutingPlan{
		Provider: service.PlatformOpenAI,
		Model:    core.ModelResolution{UpstreamModel: "gpt-5"},
	}, []*service.Account{
		{ID: 1, Platform: service.PlatformOpenAI, Status: service.StatusActive, Schedulable: true, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-4": "gpt-4"}}},
	}, nil)

	require.Nil(t, out.Account)
	require.Equal(t, 1, out.Diagnostics.RejectedBy[RejectModelUnsupported])
}

func TestSelectorReportsForcePlatformMismatch(t *testing.T) {
	out := NewSelector().Select(context.Background(), core.RoutingPlan{
		Provider: service.PlatformAntigravity,
		Meta:     map[string]any{"force_platform": service.PlatformAntigravity},
	}, []*service.Account{
		{ID: 1, Platform: service.PlatformOpenAI, Status: service.StatusActive, Schedulable: true},
	}, nil)

	require.Nil(t, out.Account)
	require.Equal(t, 1, out.Diagnostics.RejectedBy[RejectForcePlatformMismatch])
}

func TestSelectorReportsRateLimited(t *testing.T) {
	resetAt := time.Now().Add(time.Minute)
	out := NewSelector().Select(context.Background(), core.RoutingPlan{
		Provider: service.PlatformOpenAI,
	}, []*service.Account{
		{ID: 1, Platform: service.PlatformOpenAI, Status: service.StatusActive, Schedulable: true, RateLimitResetAt: &resetAt},
	}, nil)

	require.Nil(t, out.Account)
	require.Equal(t, 1, out.Diagnostics.RejectedBy[RejectRPMLimited])
}
