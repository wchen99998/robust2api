package scheduler

import "github.com/Wei-Shaw/sub2api/internal/gateway/core"

const (
	RejectPlatformMismatch      = "platform_mismatch"
	RejectModelUnsupported      = "model_unsupported"
	RejectChannelRestricted     = "channel_restricted"
	RejectExcluded              = "excluded"
	RejectUnschedulable         = "unschedulable"
	RejectQuotaExhausted        = "quota_exhausted"
	RejectRPMLimited            = "rpm_limited"
	RejectWindowCostLimited     = "window_cost_limited"
	RejectConcurrencyFull       = "concurrency_full"
	RejectStickyMismatch        = "sticky_mismatch"
	RejectForcePlatformMismatch = "force_platform_mismatch"
)

type Diagnostics struct {
	total      int
	eligible   int
	rejectedBy map[string]int
}

func NewDiagnostics(total int) *Diagnostics {
	return &Diagnostics{total: total, rejectedBy: make(map[string]int)}
}

func (d *Diagnostics) Reject(reason string) {
	if d == nil || reason == "" {
		return
	}
	d.rejectedBy[reason]++
}

func (d *Diagnostics) Accept() {
	if d == nil {
		return
	}
	d.eligible++
}

func (d *Diagnostics) Snapshot() core.CandidateDiagnostics {
	if d == nil {
		return core.CandidateDiagnostics{}
	}
	rejected := make(map[string]int, len(d.rejectedBy))
	for reason, count := range d.rejectedBy {
		rejected[reason] = count
	}
	return core.CandidateDiagnostics{
		Total:      d.total,
		Eligible:   d.eligible,
		RejectedBy: rejected,
	}
}
