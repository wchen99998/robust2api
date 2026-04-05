package logger

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestFromContextAddsTraceFields(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	base := zap.New(core)

	tp := trace.NewTracerProvider()
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})

	ctx := IntoContext(context.Background(), base)
	ctx, span := tp.Tracer("logger-test").Start(ctx, "request")
	defer span.End()

	FromContext(ctx).Info("with-trace")

	entries := observed.AllUntimed()
	require.Len(t, entries, 1)

	fields := entries[0].ContextMap()
	require.NotEmpty(t, fields["trace_id"])
	require.NotEmpty(t, fields["span_id"])
}
