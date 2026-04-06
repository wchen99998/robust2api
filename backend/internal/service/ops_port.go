package service

import (
	"context"
	"time"
)

type OpsRepository interface {
	InsertErrorLog(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error)
	BatchInsertErrorLogs(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error)
	ListErrorLogs(ctx context.Context, filter *OpsErrorLogFilter) (*OpsErrorLogList, error)
	GetErrorLogByID(ctx context.Context, id int64) (*OpsErrorLogDetail, error)
	ListRequestDetails(ctx context.Context, filter *OpsRequestDetailFilter) ([]*OpsRequestDetail, int64, error)

	InsertRetryAttempt(ctx context.Context, input *OpsInsertRetryAttemptInput) (int64, error)
	UpdateRetryAttempt(ctx context.Context, input *OpsUpdateRetryAttemptInput) error
	GetLatestRetryAttemptForError(ctx context.Context, sourceErrorID int64) (*OpsRetryAttempt, error)
	ListRetryAttemptsByErrorID(ctx context.Context, sourceErrorID int64, limit int) ([]*OpsRetryAttempt, error)
	UpdateErrorResolution(ctx context.Context, errorID int64, resolved bool, resolvedByUserID *int64, resolvedRetryID *int64, resolvedAt *time.Time) error
}

type OpsInsertErrorLogInput struct {
	RequestID       string
	ClientRequestID string

	UserID    *int64
	APIKeyID  *int64
	AccountID *int64
	GroupID   *int64
	ClientIP  *string

	Platform    string
	Model       string
	RequestPath string
	Stream      bool
	// InboundEndpoint is the normalized client-facing API endpoint path, e.g. /v1/chat/completions.
	InboundEndpoint string
	// UpstreamEndpoint is the normalized upstream endpoint path, e.g. /v1/responses.
	UpstreamEndpoint string
	// RequestedModel is the client-requested model name before mapping.
	RequestedModel string
	// UpstreamModel is the actual model sent to upstream after mapping. Empty means no mapping.
	UpstreamModel string
	// RequestType is the granular request type: 0=unknown, 1=sync, 2=stream, 3=ws_v2.
	// Matches service.RequestType enum semantics from usage_log.go.
	RequestType *int16
	UserAgent   string

	ErrorPhase        string
	ErrorType         string
	Severity          string
	StatusCode        int
	IsBusinessLimited bool
	IsCountTokens     bool // 是否为 count_tokens 请求

	ErrorMessage string
	ErrorBody    string

	ErrorSource string
	ErrorOwner  string

	UpstreamStatusCode   *int
	UpstreamErrorMessage *string
	UpstreamErrorDetail  *string
	// UpstreamErrors captures all upstream error attempts observed during handling this request.
	// It is populated during request processing (gin context) and sanitized+serialized by OpsService.
	UpstreamErrors []*OpsUpstreamErrorEvent
	// UpstreamErrorsJSON is the sanitized JSON string stored into ops_error_logs.upstream_errors.
	// It is set by OpsService.RecordError before persisting.
	UpstreamErrorsJSON *string

	AuthLatencyMs      *int64
	RoutingLatencyMs   *int64
	UpstreamLatencyMs  *int64
	ResponseLatencyMs  *int64
	TimeToFirstTokenMs *int64

	RequestBodyJSON      *string // sanitized json string (not raw bytes)
	RequestBodyTruncated bool
	RequestBodyBytes     *int
	RequestHeadersJSON   *string // optional json string

	IsRetryable bool
	RetryCount  int

	CreatedAt time.Time
}

type OpsInsertRetryAttemptInput struct {
	RequestedByUserID int64
	SourceErrorID     int64
	Mode              string
	PinnedAccountID   *int64

	// running|queued etc.
	Status    string
	StartedAt time.Time
}

type OpsUpdateRetryAttemptInput struct {
	ID int64

	// succeeded|failed
	Status     string
	FinishedAt time.Time
	DurationMs int64

	// Persisted execution results (best-effort)
	Success           *bool
	HTTPStatusCode    *int
	UpstreamRequestID *string
	UsedAccountID     *int64
	ResponsePreview   *string
	ResponseTruncated *bool

	// Optional correlation (legacy fields kept)
	ResultRequestID *string
	ResultErrorID   *int64

	ErrorMessage *string
}
