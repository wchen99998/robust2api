package domain

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"time"
)

type IngressRequest struct {
	RequestID string        `json:"request_id"`
	Endpoint  EndpointKind  `json:"endpoint"`
	Platform  Platform      `json:"platform"`
	Transport TransportKind `json:"transport"`
	Method    string        `json:"method"`
	Path      string        `json:"path"`
	Header    http.Header   `json:"header,omitempty"`
}

func (r IngressRequest) MarshalJSON() ([]byte, error) {
	type ingressRequestJSON struct {
		RequestID string        `json:"request_id"`
		Endpoint  EndpointKind  `json:"endpoint"`
		Platform  Platform      `json:"platform"`
		Transport TransportKind `json:"transport"`
		Method    string        `json:"method"`
		Path      string        `json:"path"`
		Header    http.Header   `json:"header,omitempty"`
	}

	return json.Marshal(ingressRequestJSON{
		RequestID: r.RequestID,
		Endpoint:  r.Endpoint,
		Platform:  r.Platform,
		Transport: r.Transport,
		Method:    r.Method,
		Path:      r.Path,
		Header:    redactHeaders(r.Header),
	})
}

type RoutingPlan struct {
	Request     IngressRequest       `json:"request"`
	Subject     Subject              `json:"subject"`
	Canonical   CanonicalRequest     `json:"canonical"`
	GroupID     int64                `json:"group_id"`
	Session     SessionDecision      `json:"session"`
	Diagnostics CandidateDiagnostics `json:"diagnostics"`
	Account     AccountDecision      `json:"account"`
	Retry       RetryPlan            `json:"retry"`
	Billing     BillingLifecyclePlan `json:"billing"`
	Debug       DebugPlan            `json:"debug"`
	CreatedAt   time.Time            `json:"created_at"`
}

func (p RoutingPlan) IsZero() bool {
	return reflect.DeepEqual(p, RoutingPlan{})
}

type ExecutionReport struct {
	RequestID  string                 `json:"request_id"`
	Plan       RoutingPlan            `json:"plan"`
	Attempts   []AttemptTrace         `json:"attempts,omitempty"`
	Usage      *UsageEvent            `json:"usage,omitempty"`
	Billing    BillingExecutionReport `json:"billing"`
	Error      *GatewayError          `json:"error,omitempty"`
	StartedAt  time.Time              `json:"started_at"`
	FinishedAt time.Time              `json:"finished_at"`
}

func (r ExecutionReport) Succeeded() bool {
	if r.Error != nil {
		return false
	}
	if len(r.Attempts) == 0 {
		return false
	}
	return r.Attempts[len(r.Attempts)-1].Outcome == AttemptOutcomeSuccess
}

type GatewayError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code,omitempty"`
	Retryable  bool   `json:"retryable"`
}

func redactHeaders(headers http.Header) http.Header {
	if len(headers) == 0 {
		return nil
	}

	out := make(http.Header, len(headers))
	for name, values := range headers {
		if isSensitiveHeader(name) {
			out[name] = []string{"[REDACTED]"}
			continue
		}
		out[name] = append([]string(nil), values...)
	}
	return out
}

func isSensitiveHeader(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return false
	}

	switch normalized {
	case "authorization",
		"proxy-authorization",
		"cookie",
		"set-cookie",
		"x-api-key",
		"access-token",
		"refresh-token",
		"id-token",
		"x-auth-token",
		"x-client-secret",
		"private-token",
		"session-token":
		return true
	}

	sensitiveTerms := []string{
		"api-key",
		"api_key",
		"apikey",
		"token",
		"secret",
		"credential",
		"password",
	}
	for _, term := range sensitiveTerms {
		if strings.Contains(normalized, term) {
			return true
		}
	}
	return false
}
