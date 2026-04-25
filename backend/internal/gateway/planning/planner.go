package planning

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/gateway/observability"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/tidwall/gjson"
)

var ErrUnsupportedEndpoint = errors.New("unsupported gateway endpoint")

type Planner struct {
	MaxAccountSwitches int
}

func NewPlanner(maxAccountSwitches int) *Planner {
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}
	return &Planner{MaxAccountSwitches: maxAccountSwitches}
}

func (p *Planner) Plan(req core.IngressRequest) (*core.RoutingPlan, *core.CanonicalRequest, error) {
	endpoint := req.Endpoint
	if endpoint == core.EndpointUnknown {
		endpoint = DetectEndpoint(req.Path)
	}
	if endpoint == core.EndpointUnknown {
		return nil, nil, ErrUnsupportedEndpoint
	}
	provider := ResolveProvider(req, endpoint)
	model, stream := parseModelAndStream(req.Body)
	stream = stream || req.IsWebSocket
	channelMapped := model
	billingModel := model
	if req.APIKey != nil && req.APIKey.Group != nil {
		if mapped := strings.TrimSpace(req.APIKey.Group.ResolveMessagesDispatchModel(model)); mapped != "" && endpoint == core.EndpointMessages && provider == service.PlatformOpenAI {
			channelMapped = mapped
			billingModel = mapped
		} else if mapped = strings.TrimSpace(req.APIKey.Group.DefaultMappedModel); mapped != "" && model == "" {
			channelMapped = mapped
			billingModel = mapped
		}
	}
	groupID := (*int64)(nil)
	if req.APIKey != nil {
		groupID = req.APIKey.GroupID
	}
	canonical := &core.CanonicalRequest{
		RequestID:      req.RequestID,
		Endpoint:       endpoint,
		Provider:       provider,
		RequestedModel: model,
		Stream:         stream,
		Body:           req.Body,
		Headers:        cloneHeader(req.Headers),
		Subpath:        ResponsesSubpath(req.RawPath),
		Session: core.SessionInput{
			Key:       defaultSessionKey(req),
			ClientIP:  req.ClientIP,
			UserAgent: req.Headers.Get("User-Agent"),
		},
	}
	if req.APIKey != nil {
		canonical.Session.APIKeyID = req.APIKey.ID
	}
	plan := &core.RoutingPlan{
		RequestID: req.RequestID,
		Endpoint:  endpoint,
		Provider:  provider,
		GroupID:   groupID,
		Model: core.ModelResolution{
			RequestedModel:     model,
			ChannelMappedModel: channelMapped,
			UpstreamModel:      channelMapped,
			BillingModel:       billingModel,
			BillingModelSource: "request",
		},
		Session: core.SessionDecision{Key: canonical.Session.Key},
		Retry: core.RetryPlan{
			MaxAttempts:        p.MaxAccountSwitches + 1,
			MaxAccountSwitches: p.MaxAccountSwitches,
			SameAccountRetries: 1,
			BackoffInitial:     120 * time.Millisecond,
			BackoffMax:         2 * time.Second,
		},
		Billing: core.BillingPlan{
			Enabled:            true,
			Streaming:          stream,
			RequestPayloadHash: service.HashUsageRequestPayload(req.Body),
			Model:              billingModel,
		},
		Transport: core.TransportPlan{
			Method:    req.Method,
			Stream:    stream,
			WebSocket: req.IsWebSocket,
		},
		Debug: core.DebugPlan{
			SafeHeaders: observability.SafeHeaders(req.Headers),
			BodyDigest:  observability.BodyFingerprint(req.Body),
			BodyBytes:   len(req.Body),
		},
		Meta: map[string]any{
			"force_platform": req.ForcePlatform,
			"raw_path":       req.RawPath,
			"path":           req.Path,
		},
	}
	if plan.Model.BillingModel == "" {
		plan.Model.BillingModelSource = ""
	}
	return plan, canonical, nil
}

func DetectEndpoint(path string) core.EndpointKind {
	clean := strings.TrimSpace(path)
	switch {
	case strings.Contains(clean, "/messages/count_tokens"):
		return core.EndpointCountTokens
	case strings.Contains(clean, "/messages"):
		return core.EndpointMessages
	case strings.Contains(clean, "/chat/completions"):
		return core.EndpointChatCompletions
	case strings.Contains(clean, "/responses"):
		return core.EndpointResponses
	case strings.Contains(clean, "/v1beta/models"):
		return core.EndpointGeminiModels
	case strings.HasSuffix(clean, "/v1/usage") || strings.HasSuffix(clean, "/usage"):
		return core.EndpointUsage
	case strings.HasSuffix(clean, "/v1/models") || strings.HasSuffix(clean, "/models"):
		return core.EndpointModels
	case strings.HasPrefix(clean, "/antigravity"):
		return core.EndpointAntigravity
	default:
		return core.EndpointUnknown
	}
}

func ResolveProvider(req core.IngressRequest, endpoint core.EndpointKind) string {
	if forced := strings.TrimSpace(req.ForcePlatform); forced != "" {
		return forced
	}
	if strings.HasPrefix(req.Path, "/openai/") {
		return service.PlatformOpenAI
	}
	if strings.HasPrefix(req.Path, "/antigravity/") {
		return service.PlatformAntigravity
	}
	if endpoint == core.EndpointGeminiModels {
		return service.PlatformGemini
	}
	if req.APIKey != nil && req.APIKey.Group != nil && strings.TrimSpace(req.APIKey.Group.Platform) != "" {
		return req.APIKey.Group.Platform
	}
	return service.PlatformAnthropic
}

func ResponsesSubpath(rawPath string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(rawPath), "/")
	idx := strings.LastIndex(trimmed, "/responses")
	if idx < 0 {
		return ""
	}
	suffix := trimmed[idx+len("/responses"):]
	if suffix == "" || suffix == "/" {
		return ""
	}
	return suffix
}

func parseModelAndStream(body []byte) (string, bool) {
	if !json.Valid(body) {
		return "", false
	}
	return strings.TrimSpace(gjson.GetBytes(body, "model").String()), gjson.GetBytes(body, "stream").Bool()
}

func defaultSessionKey(req core.IngressRequest) string {
	if v := strings.TrimSpace(req.Headers.Get("Session_ID")); v != "" {
		return v
	}
	if v := strings.TrimSpace(req.Headers.Get("Conversation_ID")); v != "" {
		return v
	}
	if req.APIKey != nil {
		return strings.TrimSpace(req.ClientIP) + ":" + req.Headers.Get("User-Agent")
	}
	return ""
}

func cloneHeader(headers http.Header) http.Header {
	if len(headers) == 0 {
		return nil
	}
	return headers.Clone()
}
