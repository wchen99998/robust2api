package core

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gateway/domain"
)

var (
	_ Core          = (*fakeCore)(nil)
	_ ResponseSink  = (*fakeSink)(nil)
	_ WebSocketConn = (*fakeWebSocketConn)(nil)
)

type fakeCore struct {
	httpReport      *domain.ExecutionReport
	webSocketReport *domain.ExecutionReport
}

func (c *fakeCore) ExecuteHTTP(ctx context.Context, req domain.IngressRequest, sink ResponseSink) (*domain.ExecutionReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.Transport != domain.TransportHTTP {
		return nil, errors.New("expected http transport")
	}
	if err := sink.WriteHeader(http.StatusAccepted, http.Header{"Content-Type": []string{"application/json"}}); err != nil {
		return nil, err
	}
	if err := sink.WriteChunk([]byte(`{"ok":true}`)); err != nil {
		return nil, err
	}
	if err := sink.Flush(); err != nil {
		return nil, err
	}
	return c.httpReport, nil
}

func (c *fakeCore) ExecuteWebSocket(ctx context.Context, req domain.IngressRequest, conn WebSocketConn) (*domain.ExecutionReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.Transport != domain.TransportWebSocket {
		return nil, errors.New("expected websocket transport")
	}
	messageType, data, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	if err := conn.Write(ctx, messageType, data); err != nil {
		return nil, err
	}
	if err := conn.Close(WebSocketCloseStatusNormalClosure, "done"); err != nil {
		return nil, err
	}
	return c.webSocketReport, nil
}

type fakeSink struct {
	status    int
	header    http.Header
	chunks    [][]byte
	flushed   bool
	committed bool
}

func (s *fakeSink) WriteHeader(status int, header http.Header) error {
	s.status = status
	s.header = header.Clone()
	s.committed = true
	return nil
}

func (s *fakeSink) WriteChunk(chunk []byte) error {
	s.chunks = append(s.chunks, append([]byte(nil), chunk...))
	return nil
}

func (s *fakeSink) Flush() error {
	s.flushed = true
	return nil
}

func (s *fakeSink) Committed() bool {
	return s.committed
}

type fakeWebSocketConn struct {
	readType    WebSocketMessageType
	readData    []byte
	writes      []fakeWebSocketWrite
	closeStatus WebSocketCloseStatus
	closeReason string
	closed      bool
}

type fakeWebSocketWrite struct {
	messageType WebSocketMessageType
	data        []byte
}

func (c *fakeWebSocketConn) Read(ctx context.Context) (WebSocketMessageType, []byte, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	return c.readType, append([]byte(nil), c.readData...), nil
}

func (c *fakeWebSocketConn) Write(ctx context.Context, messageType WebSocketMessageType, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.writes = append(c.writes, fakeWebSocketWrite{
		messageType: messageType,
		data:        append([]byte(nil), data...),
	})
	return nil
}

func (c *fakeWebSocketConn) Close(status WebSocketCloseStatus, reason string) error {
	c.closeStatus = status
	c.closeReason = reason
	c.closed = true
	return nil
}

func TestCoreContractsAreUsableWithoutGin(t *testing.T) {
	report := &domain.ExecutionReport{RequestID: "req-http"}
	req := domain.IngressRequest{
		RequestID: "req-http",
		Endpoint:  domain.EndpointOpenAIChatCompletions,
		Platform:  domain.PlatformOpenAI,
		Transport: domain.TransportHTTP,
		Method:    http.MethodPost,
		Path:      "/v1/chat/completions",
		Header:    http.Header{"X-Request-Id": []string{"req-http"}},
	}
	sink := &fakeSink{}
	core := Core(&fakeCore{httpReport: report})

	got, err := core.ExecuteHTTP(context.Background(), req, sink)
	if err != nil {
		t.Fatalf("ExecuteHTTP() error = %v", err)
	}
	if got != report {
		t.Fatalf("ExecuteHTTP() report = %p, want %p", got, report)
	}
	if !sink.Committed() {
		t.Fatal("sink was not committed")
	}
	if sink.status != http.StatusAccepted {
		t.Fatalf("sink status = %d, want %d", sink.status, http.StatusAccepted)
	}
	if got := sink.header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("sink content type = %q, want application/json", got)
	}
	if len(sink.chunks) != 1 || string(sink.chunks[0]) != `{"ok":true}` {
		t.Fatalf("sink chunks = %q, want one JSON chunk", sink.chunks)
	}
	if !sink.flushed {
		t.Fatal("sink was not flushed")
	}
}

func TestWebSocketContractIsUsableWithoutConcreteLibrary(t *testing.T) {
	report := &domain.ExecutionReport{RequestID: "req-ws"}
	req := domain.IngressRequest{
		RequestID: "req-ws",
		Endpoint:  domain.EndpointOpenAIResponses,
		Platform:  domain.PlatformOpenAI,
		Transport: domain.TransportWebSocket,
		Method:    http.MethodGet,
		Path:      "/v1/realtime",
	}
	conn := &fakeWebSocketConn{
		readType: WebSocketMessageTypeText,
		readData: []byte("hello"),
	}
	core := Core(&fakeCore{webSocketReport: report})

	got, err := core.ExecuteWebSocket(context.Background(), req, conn)
	if err != nil {
		t.Fatalf("ExecuteWebSocket() error = %v", err)
	}
	if got != report {
		t.Fatalf("ExecuteWebSocket() report = %p, want %p", got, report)
	}
	if len(conn.writes) != 1 {
		t.Fatalf("websocket writes = %d, want 1", len(conn.writes))
	}
	if conn.writes[0].messageType != WebSocketMessageTypeText {
		t.Fatalf("write message type = %q, want %q", conn.writes[0].messageType, WebSocketMessageTypeText)
	}
	if string(conn.writes[0].data) != "hello" {
		t.Fatalf("write data = %q, want hello", conn.writes[0].data)
	}
	if !conn.closed {
		t.Fatal("websocket connection was not closed")
	}
	if conn.closeStatus != WebSocketCloseStatusNormalClosure {
		t.Fatalf("close status = %d, want %d", conn.closeStatus, WebSocketCloseStatusNormalClosure)
	}
	if conn.closeReason != "done" {
		t.Fatalf("close reason = %q, want done", conn.closeReason)
	}
}
