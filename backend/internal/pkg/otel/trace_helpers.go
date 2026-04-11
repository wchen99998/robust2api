package otel

import (
	"strconv"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const gatewayTracerName = "robust2api.gateway"

const (
	EventFailoverCandidate = "robust2api.failover_candidate_detected"
	EventSameAccountRetry  = "robust2api.same_account_retry"
	EventAccountSwitch     = "robust2api.account_switch"
	EventUpstreamTimeout   = "robust2api.upstream_timeout"
	EventStreamIdleTimeout = "robust2api.stream_idle_timeout"
	EventClientDisconnect  = "robust2api.client_disconnect"
	EventFallbackResponse  = "robust2api.fallback_response_written"
)

func GatewayTracer() trace.Tracer {
	return otel.Tracer(gatewayTracerName)
}

func AttrPlatform(value string) attribute.KeyValue {
	return attribute.String("robust2api.platform", strings.TrimSpace(value))
}

func AttrRequestedModel(value string) attribute.KeyValue {
	return attribute.String("robust2api.requested_model", strings.TrimSpace(value))
}

func AttrEffectiveModel(value string) attribute.KeyValue {
	return attribute.String("robust2api.effective_model", strings.TrimSpace(value))
}

func AttrStream(value bool) attribute.KeyValue {
	return attribute.Bool("robust2api.stream", value)
}

func AttrUserID(value int64) attribute.KeyValue {
	return attribute.Int64("robust2api.user_id", value)
}

func AttrAPIKeyID(value int64) attribute.KeyValue {
	return attribute.Int64("robust2api.api_key_id", value)
}

func AttrGroupID(value int64) attribute.KeyValue {
	return attribute.Int64("robust2api.group_id", value)
}

func AttrAccountID(value int64) attribute.KeyValue {
	return attribute.Int64("robust2api.account_id", value)
}

func AttrTransport(value string) attribute.KeyValue {
	return attribute.String("robust2api.transport", strings.TrimSpace(value))
}

func AttrUpstreamRequestID(value string) attribute.KeyValue {
	return attribute.String("robust2api.upstream_request_id", strings.TrimSpace(value))
}

func AttrUpstreamStatusCode(value int) attribute.KeyValue {
	return attribute.Int("robust2api.upstream_status_code", value)
}

func AttrUpstreamLatencyMs(value int64) attribute.KeyValue {
	return attribute.Int64("robust2api.upstream_latency_ms", value)
}

func AttrTTFTMs(value int64) attribute.KeyValue {
	return attribute.Int64("robust2api.ttft_ms", value)
}

func AttrFailoverSwitchCount(value int) attribute.KeyValue {
	return attribute.Int("robust2api.failover_switch_count", value)
}

func AttrStatusCodeLabel(value int) attribute.KeyValue {
	return attribute.String("robust2api.status_code", strconv.Itoa(value))
}

func SetSpanAttributes(span trace.Span, attrs ...attribute.KeyValue) {
	if span == nil {
		return
	}
	filtered := attrs[:0]
	for _, attr := range attrs {
		switch v := attr.Value.AsInterface().(type) {
		case string:
			if strings.TrimSpace(v) == "" {
				continue
			}
		}
		filtered = append(filtered, attr)
	}
	if len(filtered) > 0 {
		span.SetAttributes(filtered...)
	}
}

func AddSpanEvent(span trace.Span, name string, attrs ...attribute.KeyValue) {
	if span == nil || strings.TrimSpace(name) == "" {
		return
	}
	filtered := attrs[:0]
	for _, attr := range attrs {
		switch v := attr.Value.AsInterface().(type) {
		case string:
			if strings.TrimSpace(v) == "" {
				continue
			}
		}
		filtered = append(filtered, attr)
	}
	if len(filtered) == 0 {
		span.AddEvent(name)
		return
	}
	span.AddEvent(name, trace.WithAttributes(filtered...))
}

func RecordSpanError(span trace.Span, err error, description string, attrs ...attribute.KeyValue) {
	if span == nil {
		return
	}
	if err != nil {
		span.RecordError(err)
	}
	description = strings.TrimSpace(description)
	if description == "" && err != nil {
		description = err.Error()
	}
	if description != "" {
		span.SetStatus(codes.Error, description)
	}
	if len(attrs) > 0 {
		SetSpanAttributes(span, attrs...)
	}
}
