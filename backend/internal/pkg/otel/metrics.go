package otel

import (
	"context"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const meterName = "github.com/Wei-Shaw/sub2api"

var (
	globalMetrics     *Metrics
	globalMetricsOnce sync.Once
)

// M returns the global Metrics instance, creating it on first call.
// Safe for concurrent use. Returns a no-op-safe instance (all OTel
// instruments are safe to call even without a configured provider).
func M() *Metrics {
	globalMetricsOnce.Do(func() {
		m, _ := NewMetrics() // instruments are no-op safe; error is impossible with global meter
		globalMetrics = m
	})
	return globalMetrics
}

// Metrics holds all application-level OTel metric instruments.
type Metrics struct {
	httpRequestsTotal        metric.Int64Counter
	httpRequestDuration      metric.Float64Histogram
	httpRequestTTFT          metric.Float64Histogram
	upstreamRequestDuration  metric.Float64Histogram
	tokensTotal              metric.Int64Counter
	upstreamErrorsTotal      metric.Int64Counter
	accountFailoversTotal    metric.Int64Counter
	rateLimitRejectionsTotal metric.Int64Counter
	concurrencyQueueDepth    metric.Int64Gauge
	upstreamAccountsActive   metric.Int64Gauge
	billingPublishTotal      metric.Int64Counter
	billingPublishFailures   metric.Int64Counter
	billingPublishLatency    metric.Float64Histogram
	billingApplyTotal        metric.Int64Counter
	billingApplyFailures     metric.Int64Counter
	billingPendingMessages   metric.Int64Gauge
	billingPendingOldestAge  metric.Float64Gauge
	billingDLQMessages       metric.Int64Gauge
	billingLastApplySuccess  metric.Int64Gauge
	billingLastApplyFailure  metric.Int64Gauge
}

// NewMetrics creates and registers all application metric instruments.
func NewMetrics() (*Metrics, error) {
	meter := otel.Meter(meterName)
	m := &Metrics{}
	var err error

	m.httpRequestsTotal, err = meter.Int64Counter("sub2api.http.requests",
		metric.WithDescription("Total HTTP requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	m.httpRequestDuration, err = meter.Float64Histogram("sub2api.http.request.duration",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 15, 20, 30, 60),
	)
	if err != nil {
		return nil, err
	}

	m.httpRequestTTFT, err = meter.Float64Histogram("sub2api.http.request.ttft",
		metric.WithDescription("Time to first token in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 3, 5, 10),
	)
	if err != nil {
		return nil, err
	}

	m.upstreamRequestDuration, err = meter.Float64Histogram("sub2api.upstream.request.duration",
		metric.WithDescription("Per-upstream-attempt request duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 15, 20, 30, 60),
	)
	if err != nil {
		return nil, err
	}

	m.tokensTotal, err = meter.Int64Counter("sub2api.tokens",
		metric.WithDescription("Total tokens processed"),
		metric.WithUnit("{token}"),
	)
	if err != nil {
		return nil, err
	}

	m.upstreamErrorsTotal, err = meter.Int64Counter("sub2api.upstream.errors",
		metric.WithDescription("Total upstream errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, err
	}

	m.accountFailoversTotal, err = meter.Int64Counter("sub2api.account.failovers",
		metric.WithDescription("Total account failover events"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return nil, err
	}

	m.rateLimitRejectionsTotal, err = meter.Int64Counter("sub2api.ratelimit.rejections",
		metric.WithDescription("Total rate limit rejections"),
		metric.WithUnit("{rejection}"),
	)
	if err != nil {
		return nil, err
	}

	m.concurrencyQueueDepth, err = meter.Int64Gauge("sub2api.concurrency.queue_depth",
		metric.WithDescription("Current concurrency queue depth"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	m.upstreamAccountsActive, err = meter.Int64Gauge("sub2api.upstream.accounts_active",
		metric.WithDescription("Number of active upstream accounts"),
		metric.WithUnit("{account}"),
	)
	if err != nil {
		return nil, err
	}

	m.billingPublishTotal, err = meter.Int64Counter("sub2api.billing.publish",
		metric.WithDescription("Total billing publish attempts"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return nil, err
	}

	m.billingPublishFailures, err = meter.Int64Counter("sub2api.billing.publish_failures",
		metric.WithDescription("Total failed billing publish attempts"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return nil, err
	}

	m.billingPublishLatency, err = meter.Float64Histogram("sub2api.billing.publish_latency",
		metric.WithDescription("Billing publish latency in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10),
	)
	if err != nil {
		return nil, err
	}

	m.billingApplyTotal, err = meter.Int64Counter("sub2api.billing.apply",
		metric.WithDescription("Total billing apply attempts"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return nil, err
	}

	m.billingApplyFailures, err = meter.Int64Counter("sub2api.billing.apply_failures",
		metric.WithDescription("Total failed billing apply attempts"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return nil, err
	}

	m.billingPendingMessages, err = meter.Int64Gauge("sub2api.billing.pending_messages",
		metric.WithDescription("Current number of pending billing stream messages"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return nil, err
	}

	m.billingPendingOldestAge, err = meter.Float64Gauge("sub2api.billing.pending_oldest_age",
		metric.WithDescription("Age in seconds of the oldest pending billing message"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	m.billingDLQMessages, err = meter.Int64Gauge("sub2api.billing.dlq_messages",
		metric.WithDescription("Current number of billing DLQ messages"),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return nil, err
	}

	m.billingLastApplySuccess, err = meter.Int64Gauge("sub2api.billing.last_apply_success_timestamp",
		metric.WithDescription("Unix timestamp of the last successful billing apply"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	m.billingLastApplyFailure, err = meter.Int64Gauge("sub2api.billing.last_apply_failure_timestamp",
		metric.WithDescription("Unix timestamp of the last failed billing apply"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (m *Metrics) RecordRequest(ctx context.Context, method, route string, status int, platform string) {
	attrs := []attribute.KeyValue{
		attribute.Int("http.status_code", status),
	}
	attrs = appendOptionalStringAttr(attrs, "http.method", method)
	attrs = appendOptionalStringAttr(attrs, "http.route", route)
	attrs = appendOptionalStringAttr(attrs, "platform", platform)
	m.httpRequestsTotal.Add(ctx, 1,
		metric.WithAttributes(attrs...),
	)
}

func (m *Metrics) RecordDuration(ctx context.Context, durationSec float64, method, route string, status int, platform string) {
	attrs := []attribute.KeyValue{
		attribute.Int("http.status_code", status),
	}
	attrs = appendOptionalStringAttr(attrs, "http.method", method)
	attrs = appendOptionalStringAttr(attrs, "http.route", route)
	attrs = appendOptionalStringAttr(attrs, "platform", platform)
	m.httpRequestDuration.Record(ctx, durationSec,
		metric.WithAttributes(attrs...),
	)
}

func (m *Metrics) RecordTTFT(ctx context.Context, ttftSec float64, platform, model string) {
	attrs := appendOptionalStringAttr(nil, "platform", platform)
	attrs = appendOptionalStringAttr(attrs, "model", model)
	m.httpRequestTTFT.Record(ctx, ttftSec,
		metric.WithAttributes(attrs...),
	)
}

func (m *Metrics) RecordUpstreamDuration(ctx context.Context, durationSec float64, platform, transport, statusCode, outcome string) {
	attrs := appendOptionalStringAttr(nil, "platform", platform)
	attrs = appendOptionalStringAttr(attrs, "transport", transport)
	attrs = appendOptionalStringAttr(attrs, "status_code", statusCode)
	attrs = appendOptionalStringAttr(attrs, "outcome", outcome)
	m.upstreamRequestDuration.Record(ctx, durationSec,
		metric.WithAttributes(attrs...),
	)
}

func (m *Metrics) RecordTokens(ctx context.Context, count int64, direction, platform, model string) {
	attrs := appendOptionalStringAttr(nil, "direction", direction)
	attrs = appendOptionalStringAttr(attrs, "platform", platform)
	attrs = appendOptionalStringAttr(attrs, "model", model)
	m.tokensTotal.Add(ctx, count,
		metric.WithAttributes(attrs...),
	)
}

func (m *Metrics) RecordUpstreamError(ctx context.Context, platform, errorKind string) {
	attrs := appendOptionalStringAttr(nil, "platform", platform)
	attrs = appendOptionalStringAttr(attrs, "error_kind", errorKind)
	m.upstreamErrorsTotal.Add(ctx, 1,
		metric.WithAttributes(attrs...),
	)
}

func (m *Metrics) RecordAccountFailover(ctx context.Context, platform string) {
	attrs := appendOptionalStringAttr(nil, "platform", platform)
	m.accountFailoversTotal.Add(ctx, 1,
		metric.WithAttributes(attrs...),
	)
}

func (m *Metrics) RecordRateLimitRejection(ctx context.Context, limiterType string) {
	m.rateLimitRejectionsTotal.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("limiter_type", limiterType),
		),
	)
}

func (m *Metrics) SetConcurrencyQueueDepth(ctx context.Context, depth int64) {
	m.concurrencyQueueDepth.Record(ctx, depth)
}

func (m *Metrics) SetUpstreamAccountsActive(ctx context.Context, count int64, platform string) {
	attrs := appendOptionalStringAttr(nil, "platform", platform)
	m.upstreamAccountsActive.Record(ctx, count,
		metric.WithAttributes(attrs...),
	)
}

func (m *Metrics) RecordBillingPublish(ctx context.Context, outcome string, durationSec float64) {
	attrs := appendOptionalStringAttr(nil, "outcome", outcome)
	m.billingPublishTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.billingPublishLatency.Record(ctx, durationSec, metric.WithAttributes(attrs...))
	if outcome != "success" {
		m.billingPublishFailures.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}

func (m *Metrics) RecordBillingApply(ctx context.Context, outcome string) {
	attrs := appendOptionalStringAttr(nil, "outcome", outcome)
	m.billingApplyTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	if outcome != "success" {
		m.billingApplyFailures.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}

func (m *Metrics) SetBillingPendingMessages(ctx context.Context, count int64) {
	m.billingPendingMessages.Record(ctx, count)
}

func (m *Metrics) SetBillingPendingOldestAge(ctx context.Context, ageSec float64) {
	m.billingPendingOldestAge.Record(ctx, ageSec)
}

func (m *Metrics) SetBillingDLQMessages(ctx context.Context, count int64) {
	m.billingDLQMessages.Record(ctx, count)
}

func (m *Metrics) SetBillingLastApplySuccessTimestamp(ctx context.Context, unixSec int64) {
	m.billingLastApplySuccess.Record(ctx, unixSec)
}

func (m *Metrics) SetBillingLastApplyFailureTimestamp(ctx context.Context, unixSec int64) {
	m.billingLastApplyFailure.Record(ctx, unixSec)
}

func appendOptionalStringAttr(attrs []attribute.KeyValue, key, value string) []attribute.KeyValue {
	if value = strings.TrimSpace(value); value == "" {
		return attrs
	}
	return append(attrs, attribute.String(key, value))
}
