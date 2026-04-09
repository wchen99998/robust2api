package handler

import (
	"context"

	appelotel "github.com/Wei-Shaw/sub2api/internal/pkg/otel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func startOpenAIHandlerSpan(c *gin.Context, name string) (context.Context, trace.Span) {
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	ctx, span := appelotel.GatewayTracer().Start(ctx, name)
	if c != nil && c.Request != nil {
		c.Request = c.Request.WithContext(ctx)
	}

	transport := string(service.GetOpenAIClientTransport(c))
	if transport == "" {
		transport = string(service.OpenAIClientTransportHTTP)
	}
	appelotel.SetSpanAttributes(span, appelotel.AttrTransport(transport))
	return ctx, span
}

func setOpenAIRequestSpanIdentity(span trace.Span, apiKey *service.APIKey, userID int64, requestedModel string, stream bool) {
	attrs := []attribute.KeyValue{
		appelotel.AttrUserID(userID),
		appelotel.AttrRequestedModel(requestedModel),
		appelotel.AttrStream(stream),
	}
	if apiKey != nil {
		attrs = append(attrs, appelotel.AttrAPIKeyID(apiKey.ID))
		if apiKey.GroupID != nil {
			attrs = append(attrs, appelotel.AttrGroupID(*apiKey.GroupID))
		}
	}
	appelotel.SetSpanAttributes(span, attrs...)
}

func setOpenAIAccountSpanIdentity(span trace.Span, account *service.Account, effectiveModel string) {
	if account == nil {
		return
	}
	attrs := []attribute.KeyValue{
		appelotel.AttrAccountID(account.ID),
		appelotel.AttrPlatform(account.Platform),
	}
	if effectiveModel != "" {
		attrs = append(attrs, appelotel.AttrEffectiveModel(effectiveModel))
	}
	appelotel.SetSpanAttributes(span, attrs...)
}

func openAIRequestSpanContext(parentCtx, taskCtx context.Context) context.Context {
	if taskCtx == nil {
		taskCtx = context.Background()
	}
	if spanCtx := trace.SpanContextFromContext(parentCtx); spanCtx.IsValid() {
		return trace.ContextWithSpanContext(taskCtx, spanCtx)
	}
	return taskCtx
}
