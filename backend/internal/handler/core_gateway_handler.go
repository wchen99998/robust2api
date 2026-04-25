package handler

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/gateway/ingress"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

type CoreGatewayHandler struct {
	core core.GatewayCore
}

func NewCoreGatewayHandler(core core.GatewayCore) *CoreGatewayHandler {
	return &CoreGatewayHandler{core: core}
}

func (h *CoreGatewayHandler) Handle(c *gin.Context) {
	if h == nil || h.core == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"type": "api_error", "message": "Gateway core is not configured"}})
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"type": "authentication_error", "message": "Invalid API key"}})
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"type": "api_error", "message": "User context not found"}})
		return
	}
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "Failed to read request body"}})
		return
	}
	forcedPlatform, _ := middleware2.GetForcePlatformFromContext(c)
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	user := apiKey.User
	if user == nil {
		user = &service.User{ID: subject.UserID}
	} else if user.ID == 0 {
		cloned := *user
		cloned.ID = subject.UserID
		user = &cloned
	}
	requestID, _ := c.Request.Context().Value(ctxkey.RequestID).(string)
	result, err := h.core.Handle(c.Request.Context(), core.IngressRequest{
		RequestID:     requestID,
		Method:        c.Request.Method,
		Path:          c.Request.URL.Path,
		RawPath:       c.Request.URL.EscapedPath(),
		Headers:       c.Request.Header.Clone(),
		Body:          body,
		ClientIP:      ip.GetClientIP(c),
		APIKey:        apiKey,
		User:          user,
		Subscription:  subscription,
		Endpoint:      ingress.EndpointForPath(c.Request.URL.Path),
		ForcePlatform: forcedPlatform,
		IsWebSocket:   ingress.IsWebSocket(c.Request.Header),
	})
	if err != nil {
		writeCoreGatewayError(c, http.StatusBadGateway, ingress.EndpointForPath(c.Request.URL.Path), err.Error())
		return
	}
	ingress.WriteResult(c.Writer, result)
}

func (h *CoreGatewayHandler) HandleWebSocket(c *gin.Context) {
	if h == nil || h.core == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"type": "api_error", "message": "Gateway core is not configured"}})
		return
	}
	if !ingress.IsWebSocket(c.Request.Header) {
		c.JSON(http.StatusUpgradeRequired, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "Expected WebSocket upgrade"}})
		return
	}
	wsCore, ok := h.core.(core.WebSocketGatewayCore)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"type": "api_error", "message": "Gateway core does not support WebSocket"}})
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"type": "authentication_error", "message": "Invalid API key"}})
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"type": "api_error", "message": "User context not found"}})
		return
	}
	conn, err := coderws.Accept(c.Writer, c.Request, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
	if err != nil {
		return
	}
	defer func() {
		_ = conn.CloseNow()
	}()
	forcedPlatform, _ := middleware2.GetForcePlatformFromContext(c)
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	user := apiKey.User
	if user == nil {
		user = &service.User{ID: subject.UserID}
	} else if user.ID == 0 {
		cloned := *user
		cloned.ID = subject.UserID
		user = &cloned
	}
	requestID, _ := c.Request.Context().Value(ctxkey.RequestID).(string)
	req := core.IngressRequest{
		RequestID:     requestID,
		Method:        c.Request.Method,
		Path:          c.Request.URL.Path,
		RawPath:       c.Request.URL.EscapedPath(),
		Headers:       c.Request.Header.Clone(),
		ClientIP:      ip.GetClientIP(c),
		APIKey:        apiKey,
		User:          user,
		Subscription:  subscription,
		Endpoint:      ingress.EndpointForPath(c.Request.URL.Path),
		ForcePlatform: forcedPlatform,
		IsWebSocket:   true,
	}
	if err := wsCore.HandleWebSocket(c.Request.Context(), req, coderWebSocketConn{conn: conn}); err != nil {
		_ = conn.Close(coderws.StatusInternalError, "gateway websocket error")
	}
}

type coderWebSocketConn struct {
	conn *coderws.Conn
}

func (c coderWebSocketConn) Read(ctx context.Context) (core.WebSocketMessageType, []byte, error) {
	typ, payload, err := c.conn.Read(ctx)
	if err != nil {
		return core.WebSocketMessageText, nil, err
	}
	if typ == coderws.MessageBinary {
		return core.WebSocketMessageBinary, payload, nil
	}
	return core.WebSocketMessageText, payload, nil
}

func (c coderWebSocketConn) Write(ctx context.Context, typ core.WebSocketMessageType, payload []byte) error {
	if typ == core.WebSocketMessageBinary {
		return c.conn.Write(ctx, coderws.MessageBinary, payload)
	}
	return c.conn.Write(ctx, coderws.MessageText, payload)
}

func (c coderWebSocketConn) Close(status int, reason string) error {
	return c.conn.Close(coderws.StatusCode(status), reason)
}

func writeCoreGatewayError(c *gin.Context, status int, endpoint core.EndpointKind, message string) {
	switch endpoint {
	case core.EndpointGeminiModels:
		c.JSON(status, gin.H{"error": gin.H{"code": status, "message": message, "status": http.StatusText(status)}})
	case core.EndpointMessages, core.EndpointCountTokens:
		c.JSON(status, gin.H{"type": "error", "error": gin.H{"type": "api_error", "message": message}})
	default:
		c.JSON(status, gin.H{"error": gin.H{"type": "api_error", "message": message}})
	}
}
