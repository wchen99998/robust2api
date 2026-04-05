package middleware

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

// TraceIDHeader adds the X-Trace-Id response header from the OTel span context.
// The header is set BEFORE c.Next() so that streaming responses (SSE) include it
// in the initial header flush.
func TraceIDHeader() gin.HandlerFunc {
	return func(c *gin.Context) {
		sc := trace.SpanFromContext(c.Request.Context()).SpanContext()
		if sc.HasTraceID() {
			c.Header("X-Trace-Id", sc.TraceID().String())
		}
		c.Next()
	}
}
