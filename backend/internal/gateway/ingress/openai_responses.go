package ingress

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type OpenAIResponsesPayload struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
}

func NewHTTPRequestIngress(r *http.Request, body []byte, clientIP string, apiKey *service.APIKey, user *service.User) core.IngressRequest {
	request := core.IngressRequest{
		Headers:  http.Header{},
		Body:     append([]byte(nil), body...),
		ClientIP: clientIP,
		APIKey:   apiKey,
		User:     user,
		Endpoint: core.EndpointUnknown,
	}
	if r == nil {
		return request
	}
	request.Method = r.Method
	if r.URL != nil {
		request.Path = r.URL.Path
	}
	request.Headers = r.Header.Clone()
	request.RequestID = request.Headers.Get("X-Request-ID")
	request.Endpoint = NormalizeEndpoint(request.Method, request.Path)
	return request
}

func ParseOpenAIResponses(req core.IngressRequest, session core.SessionInput) (*core.CanonicalRequest, error) {
	endpoint := req.Endpoint
	if endpoint == "" || endpoint == core.EndpointUnknown {
		endpoint = NormalizeEndpoint(req.Method, req.Path)
	}
	if endpoint != core.EndpointResponses && endpoint != core.EndpointResponsesWebSocket {
		return nil, fmt.Errorf("unsupported OpenAI responses endpoint: %s", endpoint)
	}
	payload, err := parseOpenAIResponsesPayload(req.Body)
	if err != nil {
		return nil, err
	}
	headers := req.Headers.Clone()
	if session.Key == "" {
		session.Key = firstHeader(headers, "OpenAI-Beta-Session", "X-OpenAI-Session", "Session-Id")
	}
	if session.ClientIP == "" {
		session.ClientIP = req.ClientIP
	}
	if session.UserAgent == "" {
		session.UserAgent = headers.Get("User-Agent")
	}
	if session.APIKeyID == 0 && req.APIKey != nil {
		session.APIKeyID = req.APIKey.ID
	}
	return &core.CanonicalRequest{
		RequestID:      req.RequestID,
		Endpoint:       endpoint,
		Provider:       service.PlatformOpenAI,
		RequestedModel: payload.Model,
		Stream:         payload.Stream || endpoint == core.EndpointResponsesWebSocket,
		Body:           append([]byte(nil), req.Body...),
		Parsed:         payload,
		Session:        session,
		Headers:        headers,
	}, nil
}

func parseOpenAIResponsesPayload(body []byte) (OpenAIResponsesPayload, error) {
	var payload OpenAIResponsesPayload
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			return payload, fmt.Errorf("parse OpenAI responses body: %w", err)
		}
	}
	if payload.Model == "" {
		return payload, errors.New("model is required")
	}
	return payload, nil
}

func firstHeader(headers http.Header, keys ...string) string {
	for _, key := range keys {
		if value := headers.Get(key); value != "" {
			return value
		}
	}
	return ""
}
