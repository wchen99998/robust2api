package service

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	appelotel "github.com/wchen99998/robust2api/internal/pkg/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func openAITransportLabel(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "http"
	}
	return raw
}

func openAIStatusCodeLabel(statusCode int) string {
	if statusCode <= 0 {
		return "0"
	}
	return strconv.Itoa(statusCode)
}

func openAIIsTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func openAIAttemptOutcome(statusCode int, err error, failover bool) string {
	switch {
	case err != nil && openAIIsTimeoutErr(err):
		return "timeout"
	case err != nil:
		return "request_error"
	case failover:
		return "failover"
	case statusCode >= 400:
		return "http_error"
	default:
		return "success"
	}
}

func startOpenAIUpstreamAttemptSpan(
	ctx context.Context,
	account *Account,
	requestedModel string,
	effectiveModel string,
	transport string,
) (context.Context, trace.Span, time.Time) {
	ctx, span := appelotel.GatewayTracer().Start(ctx, "gateway.upstream_attempt")
	attrs := []attribute.KeyValue{
		appelotel.AttrTransport(openAITransportLabel(transport)),
		appelotel.AttrRequestedModel(requestedModel),
		appelotel.AttrEffectiveModel(effectiveModel),
	}
	if account != nil {
		attrs = append(attrs,
			appelotel.AttrAccountID(account.ID),
			appelotel.AttrPlatform(account.Platform),
		)
	}
	appelotel.SetSpanAttributes(span, attrs...)
	return ctx, span, time.Now()
}

func finishOpenAIUpstreamAttemptSpan(
	ctx context.Context,
	span trace.Span,
	account *Account,
	transport string,
	startedAt time.Time,
	statusCode int,
	outcome string,
	upstreamRequestID string,
	err error,
) {
	if span == nil {
		return
	}
	duration := time.Since(startedAt)
	appelotel.SetSpanAttributes(span,
		appelotel.AttrUpstreamLatencyMs(duration.Milliseconds()),
		appelotel.AttrUpstreamStatusCode(statusCode),
		appelotel.AttrUpstreamRequestID(upstreamRequestID),
		attribute.String("robust2api.upstream_outcome", strings.TrimSpace(outcome)),
	)
	if err != nil {
		sanitized := sanitizeUpstreamErrorMessage(err.Error())
		appelotel.RecordSpanError(span, err, sanitized)
		if openAIIsTimeoutErr(err) {
			appelotel.AddSpanEvent(span, appelotel.EventUpstreamTimeout)
		}
	}
	if account != nil {
		appelotel.M().RecordUpstreamDuration(
			ctx,
			duration.Seconds(),
			account.Platform,
			openAITransportLabel(transport),
			openAIStatusCodeLabel(statusCode),
			strings.TrimSpace(outcome),
		)
	}
	span.End()
}
