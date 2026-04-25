package core

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
)

type Core interface {
	ExecuteHTTP(ctx context.Context, req domain.IngressRequest, sink ResponseSink) (*domain.ExecutionReport, error)
	ExecuteWebSocket(ctx context.Context, req domain.IngressRequest, conn WebSocketConn) (*domain.ExecutionReport, error)
}

type ResponseSink interface {
	WriteHeader(status int, header http.Header) error
	WriteChunk(chunk []byte) error
	Flush() error
	Committed() bool
}

type WebSocketMessageType string

const (
	WebSocketMessageTypeText   WebSocketMessageType = "text"
	WebSocketMessageTypeBinary WebSocketMessageType = "binary"
	WebSocketMessageTypeClose  WebSocketMessageType = "close"
)

type WebSocketCloseStatus int

const (
	WebSocketCloseStatusNormalClosure       WebSocketCloseStatus = 1000
	WebSocketCloseStatusGoingAway           WebSocketCloseStatus = 1001
	WebSocketCloseStatusProtocolError       WebSocketCloseStatus = 1002
	WebSocketCloseStatusUnsupportedData     WebSocketCloseStatus = 1003
	WebSocketCloseStatusInternalServerError WebSocketCloseStatus = 1011
)

type WebSocketConn interface {
	Read(ctx context.Context) (WebSocketMessageType, []byte, error)
	Write(ctx context.Context, messageType WebSocketMessageType, data []byte) error
	Close(status WebSocketCloseStatus, reason string) error
}
