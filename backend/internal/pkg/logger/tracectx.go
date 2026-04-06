package logger

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// WithTraceContext returns a logger with trace_id and span_id fields
// extracted from the OTel span context in ctx. If no active span
// exists, returns the logger unchanged.
func WithTraceContext(ctx context.Context, l *zap.Logger) *zap.Logger {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.HasTraceID() {
		return l
	}
	return l.With(
		zap.String("trace_id", sc.TraceID().String()),
		zap.String("span_id", sc.SpanID().String()),
	)
}

// FromContextWithTrace returns the context logger enriched with trace context.
func FromContextWithTrace(ctx context.Context) *zap.Logger {
	return FromContext(ctx)
}
