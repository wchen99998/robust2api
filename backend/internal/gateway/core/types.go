package core

import (
	"context"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// EndpointKind is the public gateway surface normalized into a stable routing key.
type EndpointKind string

const (
	EndpointUnknown         EndpointKind = ""
	EndpointMessages        EndpointKind = "/v1/messages"
	EndpointCountTokens     EndpointKind = "/v1/messages/count_tokens"
	EndpointModels          EndpointKind = "/v1/models"
	EndpointUsage           EndpointKind = "/v1/usage"
	EndpointResponses       EndpointKind = "/v1/responses"
	EndpointChatCompletions EndpointKind = "/v1/chat/completions"
	EndpointGeminiModels    EndpointKind = "/v1beta/models"
	EndpointAntigravity     EndpointKind = "/antigravity"
)

// GatewayCore is the only execution boundary handlers should call.
type GatewayCore interface {
	Handle(ctx context.Context, req IngressRequest) (*GatewayResult, error)
}

type UsageExtractor interface {
	Extract(ctx context.Context, plan RoutingPlan, result *GatewayResult) UsageEvent
}

type WebSocketGatewayCore interface {
	HandleWebSocket(ctx context.Context, req IngressRequest, client WebSocketConn) error
}

type WebSocketMessageType int

const (
	WebSocketMessageText   WebSocketMessageType = 1
	WebSocketMessageBinary WebSocketMessageType = 2
)

type WebSocketConn interface {
	Read(ctx context.Context) (WebSocketMessageType, []byte, error)
	Write(ctx context.Context, typ WebSocketMessageType, payload []byte) error
	Close(status int, reason string) error
}

// IngressRequest is the HTTP-server-neutral request shape accepted by the
// gateway core. Gin-specific state must be translated before constructing it.
type IngressRequest struct {
	RequestID     string
	Method        string
	Path          string
	RawPath       string
	Headers       http.Header
	Body          []byte
	ClientIP      string
	APIKey        *service.APIKey
	User          *service.User
	Subscription  *service.UserSubscription
	Endpoint      EndpointKind
	ForcePlatform string
	IsWebSocket   bool
}

func (r IngressRequest) APIKeyID() int64 {
	if r.APIKey == nil {
		return 0
	}
	return r.APIKey.ID
}

func (r IngressRequest) UserID() int64 {
	if r.User != nil {
		return r.User.ID
	}
	if r.APIKey != nil && r.APIKey.User != nil {
		return r.APIKey.User.ID
	}
	return 0
}

// CanonicalRequest is the parsed request shape shared by planning, scheduling,
// provider adapters, transport, and usage extraction.
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
	Subpath        string
}

type SessionInput struct {
	Key       string `json:"key,omitempty"`
	ClientIP  string `json:"client_ip,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	APIKeyID  int64  `json:"api_key_id,omitempty"`
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
	Transport  TransportPlan        `json:"transport"`
	Debug      DebugPlan            `json:"debug"`
	Meta       map[string]any       `json:"meta,omitempty"`
}

type ModelResolution struct {
	RequestedModel     string `json:"requested_model,omitempty"`
	ChannelMappedModel string `json:"channel_mapped_model,omitempty"`
	AccountMappedModel string `json:"account_mapped_model,omitempty"`
	UpstreamModel      string `json:"upstream_model,omitempty"`
	BillingModel       string `json:"billing_model,omitempty"`
	BillingModelSource string `json:"billing_model_source,omitempty"`
}

type SessionDecision struct {
	Key             string `json:"key,omitempty"`
	StickyAccountID *int64 `json:"sticky_account_id,omitempty"`
	StickyHit       bool   `json:"sticky_hit,omitempty"`
	StickyBound     bool   `json:"sticky_bound,omitempty"`
}

type CandidateDiagnostics struct {
	Total      int            `json:"total"`
	Eligible   int            `json:"eligible"`
	RejectedBy map[string]int `json:"rejected_by,omitempty"`
}

type AccountDecision struct {
	AccountID      int64  `json:"account_id,omitempty"`
	Platform       string `json:"platform,omitempty"`
	Type           string `json:"type,omitempty"`
	Acquired       bool   `json:"acquired,omitempty"`
	StickySelected bool   `json:"sticky_selected,omitempty"`
	Attempt        int    `json:"attempt,omitempty"`
}

type RetryPlan struct {
	MaxAttempts        int           `json:"max_attempts"`
	MaxAccountSwitches int           `json:"max_account_switches"`
	SameAccountRetries int           `json:"same_account_retries"`
	BackoffInitial     time.Duration `json:"backoff_initial,omitempty"`
	BackoffMax         time.Duration `json:"backoff_max,omitempty"`
}

type BillingPlan struct {
	Enabled            bool   `json:"enabled"`
	Streaming          bool   `json:"streaming"`
	RequestPayloadHash string `json:"request_payload_hash,omitempty"`
	Model              string `json:"model,omitempty"`
}

type TransportPlan struct {
	Method      string `json:"method,omitempty"`
	UpstreamURL string `json:"upstream_url,omitempty"`
	Stream      bool   `json:"stream"`
	WebSocket   bool   `json:"websocket"`
}

type DebugPlan struct {
	SafeHeaders map[string][]string `json:"safe_headers,omitempty"`
	BodyDigest  string              `json:"body_digest,omitempty"`
	BodyBytes   int                 `json:"body_bytes,omitempty"`
}

type UpstreamRequest struct {
	Method      string
	URL         string
	Headers     http.Header
	Body        []byte
	ProxyURL    string
	AccountID   int64
	Concurrency int
}

type UpstreamResult struct {
	StatusCode    int
	Headers       http.Header
	Body          []byte
	Stream        bool
	Duration      time.Duration
	FirstTokenMs  *int
	UpstreamError error
}

type GatewayResult struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	Stream     bool
	Usage      UsageEvent
	Plan       *RoutingPlan
}

type UpstreamErrorDecision struct {
	Retryable            bool
	RetrySameAccount     bool
	FailoverAccount      bool
	TemporaryUnschedule  bool
	ClientStatusOverride int
}

type UsageEvent struct {
	RequestID          string        `json:"request_id"`
	APIKeyID           int64         `json:"api_key_id,omitempty"`
	UserID             int64         `json:"user_id,omitempty"`
	AccountID          int64         `json:"account_id,omitempty"`
	Model              string        `json:"model,omitempty"`
	UpstreamModel      string        `json:"upstream_model,omitempty"`
	Stream             bool          `json:"stream"`
	InputTokens        int           `json:"input_tokens,omitempty"`
	OutputTokens       int           `json:"output_tokens,omitempty"`
	CacheCreateTokens  int           `json:"cache_create_tokens,omitempty"`
	CacheReadTokens    int           `json:"cache_read_tokens,omitempty"`
	ImageOutputTokens  int           `json:"image_output_tokens,omitempty"`
	RequestPayloadHash string        `json:"request_payload_hash,omitempty"`
	Duration           time.Duration `json:"duration,omitempty"`
}

type ProviderAdapter interface {
	Provider() string
	Parse(ctx context.Context, req IngressRequest) (*CanonicalRequest, error)
	Prepare(ctx context.Context, plan RoutingPlan, account *service.Account) (*UpstreamRequest, error)
	Decode(ctx context.Context, upstream *UpstreamResult) (*GatewayResult, error)
	ClassifyError(ctx context.Context, upstream *UpstreamResult) UpstreamErrorDecision
}
