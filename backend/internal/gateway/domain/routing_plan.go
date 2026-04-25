package domain

import (
	"net/http"
	"reflect"
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
	Attempts   []AttemptTrace         `json:"attempts,omitempty"`
	Usage      UsageEvent             `json:"usage"`
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
