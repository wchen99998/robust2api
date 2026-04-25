package domain

import "time"

type RejectionReason string

const (
	RejectionReasonPlatformMismatch  RejectionReason = "platform_mismatch"
	RejectionReasonModelUnsupported  RejectionReason = "model_unsupported"
	RejectionReasonChannelRestricted RejectionReason = "channel_restricted"
	RejectionReasonExcluded          RejectionReason = "excluded"
	RejectionReasonUnschedulable     RejectionReason = "unschedulable"
	RejectionReasonQuotaExhausted    RejectionReason = "quota_exhausted"
	RejectionReasonRPMLimited        RejectionReason = "rpm_limited"
	RejectionReasonWindowCostLimited RejectionReason = "window_cost_limited"
	RejectionReasonConcurrencyFull   RejectionReason = "concurrency_full"
	RejectionReasonStickyMismatch    RejectionReason = "sticky_mismatch"
	RejectionReasonTransportMismatch RejectionReason = "transport_mismatch"
)

type CandidateDiagnostics struct {
	Total       int                     `json:"total"`
	Eligible    int                     `json:"eligible"`
	Rejected    int                     `json:"rejected"`
	RejectCount map[RejectionReason]int `json:"reject_count,omitempty"`
	Samples     []CandidateSample       `json:"samples,omitempty"`
}

type CandidateSample struct {
	AccountID  int64           `json:"account_id"`
	Reason     RejectionReason `json:"reason"`
	Message    string          `json:"message,omitempty"`
	RetryAfter time.Duration   `json:"retry_after,omitempty"`
}

type RetryPlan struct {
	MaxAttempts        int           `json:"max_attempts"`
	RetrySameAccount   bool          `json:"retry_same_account"`
	RetryOtherAccounts bool          `json:"retry_other_accounts"`
	RetryableStatuses  []int         `json:"retryable_statuses,omitempty"`
	Backoff            time.Duration `json:"backoff,omitempty"`
}

type AttemptOutcome string

const (
	AttemptOutcomeUnknown          AttemptOutcome = "unknown"
	AttemptOutcomeSuccess          AttemptOutcome = "success"
	AttemptOutcomeRetryAccount     AttemptOutcome = "retry_account"
	AttemptOutcomeRetrySameAccount AttemptOutcome = "retry_same_account"
	AttemptOutcomeNonRetryable     AttemptOutcome = "non_retryable"
	AttemptOutcomeClientCanceled   AttemptOutcome = "client_canceled"
)

type AttemptTrace struct {
	Attempt      int            `json:"attempt"`
	AccountID    int64          `json:"account_id"`
	Outcome      AttemptOutcome `json:"outcome"`
	StatusCode   int            `json:"status_code,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	StartedAt    time.Time      `json:"started_at"`
	FinishedAt   time.Time      `json:"finished_at"`
	Duration     time.Duration  `json:"duration"`
}

type DebugPlan struct {
	Enabled         bool            `json:"enabled"`
	BodyFingerprint BodyFingerprint `json:"body_fingerprint"`
}

type BodyFingerprint struct {
	SHA256 string `json:"sha256,omitempty"`
	Bytes  int64  `json:"bytes"`
}
