package core

import (
	"context"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type EndpointKind string

const (
	EndpointUnknown             EndpointKind = "unknown"
	EndpointMessages            EndpointKind = "messages"
	EndpointChatCompletions     EndpointKind = "chat_completions"
	EndpointResponses           EndpointKind = "responses"
	EndpointResponsesWebSocket  EndpointKind = "responses_websocket"
	EndpointMessagesCountTokens EndpointKind = "messages_count_tokens"
	EndpointGeminiNative        EndpointKind = "gemini_native"
)

type GatewayCore interface {
	Handle(ctx context.Context, req IngressRequest) (*GatewayResult, error)
}

type IngressRequest struct {
	RequestID string
	Method    string
	Path      string
	Headers   http.Header
	Body      []byte
	ClientIP  string
	APIKey    *service.APIKey
	User      *service.User
	Endpoint  EndpointKind
}

type CanonicalRequest struct {
	RequestID      string
	Endpoint       EndpointKind
	Provider       string
	RequestedModel string
	Stream         bool
	Body           []byte
	Parsed         any
	Session        SessionInput
	Headers        http.Header
}

type SessionInput struct {
	Key            string `json:"key,omitempty"`
	MetadataUserID string `json:"metadata_user_id,omitempty"`
	ClientIP       string `json:"client_ip,omitempty"`
	UserAgent      string `json:"user_agent,omitempty"`
	APIKeyID       int64  `json:"api_key_id,omitempty"`
}

type SessionDecision struct {
	Key             string `json:"key,omitempty"`
	StickyEligible  bool   `json:"sticky_eligible"`
	CachedAccountID int64  `json:"cached_account_id,omitempty"`
	Source          string `json:"source,omitempty"`
}

type RoutingPlan struct {
	RequestID  string               `json:"request_id"`
	Endpoint   EndpointKind         `json:"endpoint"`
	Provider   string               `json:"provider"`
	Model      ModelResolution      `json:"model"`
	GroupID    *int64               `json:"group_id,omitempty"`
	Session    SessionDecision      `json:"session"`
	Candidates CandidateDiagnostics `json:"candidates"`
	Account    AccountDecision      `json:"account"`
	Retry      RetryPlan            `json:"retry"`
	Billing    BillingPlan          `json:"billing"`
	Debug      DebugPlan            `json:"debug"`
}

type ModelResolution struct {
	RequestedModel     string `json:"requested_model"`
	ChannelID          int64  `json:"channel_id,omitempty"`
	ChannelMappedModel string `json:"channel_mapped_model,omitempty"`
	AccountMappedModel string `json:"account_mapped_model,omitempty"`
	UpstreamModel      string `json:"upstream_model,omitempty"`
	BillingModel       string `json:"billing_model,omitempty"`
	BillingModelSource string `json:"billing_model_source,omitempty"`
	Restricted         bool   `json:"restricted,omitempty"`
}

type RejectionReason string

const (
	RejectedPlatformMismatch RejectionReason = "platform_mismatch"
	RejectedModelUnsupported RejectionReason = "model_unsupported"
	RejectedChannelPricing   RejectionReason = "channel_pricing_restricted"
	RejectedExcluded         RejectionReason = "excluded"
	RejectedUnschedulable    RejectionReason = "unschedulable"
	RejectedQuotaExhausted   RejectionReason = "quota_exhausted"
	RejectedRPMLimited       RejectionReason = "rpm_limited"
	RejectedWindowCost       RejectionReason = "window_cost_limited"
	RejectedConcurrencyFull  RejectionReason = "concurrency_full"
	RejectedStickyMismatch   RejectionReason = "sticky_mismatch"
)

type CandidateDiagnostics struct {
	RequestedPlatform string                      `json:"requested_platform,omitempty"`
	Total             int                         `json:"total"`
	Eligible          int                         `json:"eligible"`
	TopK              int                         `json:"top_k,omitempty"`
	LatencyMs         int64                       `json:"latency_ms,omitempty"`
	LoadSkew          float64                     `json:"load_skew,omitempty"`
	Rejected          map[RejectionReason]int     `json:"rejected,omitempty"`
	Samples           map[RejectionReason][]int64 `json:"samples,omitempty"`
}

type AccountDecision struct {
	AccountID      int64  `json:"account_id,omitempty"`
	AccountName    string `json:"account_name,omitempty"`
	Platform       string `json:"platform,omitempty"`
	Acquired       bool   `json:"acquired"`
	WaitAllowed    bool   `json:"wait_allowed"`
	SelectionMode  string `json:"selection_mode,omitempty"`
	StickySelected bool   `json:"sticky_selected"`
}

type RetryPlan struct {
	MaxAttempts    int           `json:"max_attempts"`
	TotalBudget    time.Duration `json:"total_budget"`
	BackoffInitial time.Duration `json:"backoff_initial,omitempty"`
	BackoffMax     time.Duration `json:"backoff_max,omitempty"`
	SameAccount    bool          `json:"same_account"`
}

type BillingPlan struct {
	Enabled            bool   `json:"enabled"`
	Model              string `json:"model,omitempty"`
	ModelSource        string `json:"model_source,omitempty"`
	IdempotencyKey     string `json:"idempotency_key,omitempty"`
	PayloadFingerprint string `json:"payload_fingerprint,omitempty"`
}

type DebugPlan struct {
	Enabled         bool              `json:"enabled"`
	HeaderPreview   map[string]string `json:"header_preview,omitempty"`
	BodyFingerprint string            `json:"body_fingerprint,omitempty"`
}

type UpstreamRequest struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
	Stream  bool
}

type UpstreamResult struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	Duration   time.Duration
}

type GatewayResult struct {
	RequestID string
	Status    int
	Headers   http.Header
	Body      []byte
	Stream    bool
	Plan      *RoutingPlan
	Usage     *UsageEvent
}

type UsageEvent struct {
	RequestID             string
	APIKeyID              int64
	UserID                int64
	AccountID             int64
	Provider              string
	RequestedModel        string
	BillingModel          string
	InputTokens           int64
	OutputTokens          int64
	CacheCreationTokens   int64
	CacheReadTokens       int64
	ImageOutputTokens     int64
	Status                string
	StartedAt             time.Time
	CompletedAt           time.Time
	PayloadFingerprint    string
	BillingIdempotencyKey string
}

type UpstreamErrorDecision struct {
	Retryable            bool
	RetrySameAccount     bool
	Failover             bool
	ClientStatus         int
	ClientErrorType      string
	ClientErrorMessage   string
	UpstreamStatus       int
	SanitizedUpstreamMsg string
}

type ProviderAdapter interface {
	Provider() string
	Parse(ctx context.Context, req IngressRequest) (*CanonicalRequest, error)
	Prepare(ctx context.Context, plan RoutingPlan, account *service.Account) (*UpstreamRequest, error)
	Decode(ctx context.Context, upstream *UpstreamResult) (*GatewayResult, error)
	ClassifyError(ctx context.Context, upstream *UpstreamResult) UpstreamErrorDecision
}
