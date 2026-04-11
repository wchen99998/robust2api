package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wchen99998/robust2api/internal/config"
	appelotel "github.com/wchen99998/robust2api/internal/pkg/otel"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestOpenAIGatewayService_Forward_CreatesAttemptAndTransformSpans(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousProvider := otel.GetTracerProvider()
	recorder, traceProvider := newSpanRecorderProvider()
	defer func() {
		require.NoError(t, traceProvider.Shutdown(context.Background()))
		otel.SetTracerProvider(previousProvider)
	}()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "unit-test-agent/1.0")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"rid-forward"}}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_forward_trace"}`))}},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
	}
	account := &Account{
		ID:          501,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "http://openai.internal",
		},
	}

	ctx, rootSpan := appelotel.GatewayTracer().Start(context.Background(), "gateway.responses")
	c.Request = c.Request.WithContext(ctx)
	result, err := svc.Forward(ctx, c, account, []byte(`{"model":"gpt-5.1","stream":false,"input":[{"type":"input_text","text":"hello"}]}`))
	rootSpan.End()

	require.NoError(t, err)
	require.NotNil(t, result)

	root := findEndedSpanByName(t, recorder.Ended(), "gateway.responses")
	attempt := findEndedSpanByName(t, recorder.Ended(), "gateway.upstream_attempt")
	transform := findEndedSpanByName(t, recorder.Ended(), "gateway.response_transform")

	require.Equal(t, root.SpanContext().SpanID(), attempt.Parent().SpanID())
	require.Equal(t, root.SpanContext().SpanID(), transform.Parent().SpanID())
	require.Equal(t, "http", spanAttributeValue(attempt, "robust2api.transport"))
	require.Equal(t, "openai", spanAttributeValue(attempt, "robust2api.platform"))
}

func TestOpenAIGatewayService_ForwardAsChatCompletions_CreatesAttemptAndTransformSpans(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousProvider := otel.GetTracerProvider()
	recorder, traceProvider := newSpanRecorderProvider()
	defer func() {
		require.NoError(t, traceProvider.Shutdown(context.Background()))
		otel.SetTracerProvider(previousProvider)
	}()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)
	c.Request.Header.Set("User-Agent", "unit-test-agent/1.0")

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true

	upstreamBody := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_chat_trace","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"rid-chat"}}, Body: io.NopCloser(strings.NewReader(upstreamBody))}},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
	}
	account := &Account{
		ID:          502,
		Name:        "openai-apikey-chat",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "http://openai.internal",
		},
	}

	ctx, rootSpan := appelotel.GatewayTracer().Start(context.Background(), "gateway.chat_completions")
	c.Request = c.Request.WithContext(ctx)
	result, err := svc.ForwardAsChatCompletions(ctx, c, account, body, "", "")
	rootSpan.End()

	require.NoError(t, err)
	require.NotNil(t, result)

	root := findEndedSpanByName(t, recorder.Ended(), "gateway.chat_completions")
	attempt := findEndedSpanByName(t, recorder.Ended(), "gateway.upstream_attempt")
	transform := findEndedSpanByName(t, recorder.Ended(), "gateway.response_transform")

	require.Equal(t, root.SpanContext().SpanID(), attempt.Parent().SpanID())
	require.Equal(t, root.SpanContext().SpanID(), transform.Parent().SpanID())
	require.Equal(t, "http", spanAttributeValue(attempt, "robust2api.transport"))
	require.Equal(t, "gpt-5.4", spanAttributeValue(attempt, "robust2api.requested_model"))
}

func TestOpenAIGatewayService_HandleStreamingResponse_AddsTimeoutAndDisconnectEvents(t *testing.T) {
	t.Run("stream idle timeout", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		previousProvider := otel.GetTracerProvider()
		recorder, traceProvider := newSpanRecorderProvider()
		defer func() {
			require.NoError(t, traceProvider.Shutdown(context.Background()))
			otel.SetTracerProvider(previousProvider)
		}()

		cfg := &config.Config{
			Gateway: config.GatewayConfig{
				StreamDataIntervalTimeout: 1,
				StreamKeepaliveInterval:   0,
				MaxLineSize:               defaultMaxLineSize,
			},
		}
		svc := &OpenAIGatewayService{cfg: cfg}

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

		pr, pw := io.Pipe()
		resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{}}

		ctx, rootSpan := appelotel.GatewayTracer().Start(context.Background(), "gateway.responses")
		_, err := svc.handleStreamingResponse(ctx, resp, c, &Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "gpt-5.1", "gpt-5.1")
		_ = pw.Close()
		_ = pr.Close()
		rootSpan.End()

		require.Error(t, err)
		root := findEndedSpanByName(t, recorder.Ended(), "gateway.responses")
		require.True(t, spanHasEvent(root, appelotel.EventStreamIdleTimeout))
	})

	t.Run("client disconnect", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		previousProvider := otel.GetTracerProvider()
		recorder, traceProvider := newSpanRecorderProvider()
		defer func() {
			require.NoError(t, traceProvider.Shutdown(context.Background()))
			otel.SetTracerProvider(previousProvider)
		}()

		cfg := &config.Config{
			Gateway: config.GatewayConfig{
				StreamDataIntervalTimeout: 0,
				StreamKeepaliveInterval:   0,
				MaxLineSize:               defaultMaxLineSize,
			},
		}
		svc := &OpenAIGatewayService{cfg: cfg}

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		ctx, rootSpan := appelotel.GatewayTracer().Start(context.Background(), "gateway.responses")
		c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil).WithContext(ctx)
		c.Writer = &failingGinWriter{ResponseWriter: c.Writer, failAfter: 0}

		pr, pw := io.Pipe()
		resp := &http.Response{StatusCode: http.StatusOK, Body: pr, Header: http.Header{}}
		go func() {
			defer func() { _ = pw.Close() }()
			_, _ = pw.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"))
		}()

		_, err := svc.handleStreamingResponse(ctx, resp, c, &Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "gpt-5.1", "gpt-5.1")
		_ = pr.Close()
		rootSpan.End()

		require.NoError(t, err)
		root := findEndedSpanByName(t, recorder.Ended(), "gateway.responses")
		require.True(t, spanHasEvent(root, appelotel.EventClientDisconnect))
	})
}

func TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_CreatesAttemptSpan(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousProvider := otel.GetTracerProvider()
	recorder, traceProvider := newSpanRecorderProvider()
	defer func() {
		require.NoError(t, traceProvider.Shutdown(context.Background()))
		otel.SetTracerProvider(previousProvider)
	}()

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

	captureConn := &openAIWSCaptureConn{
		events: [][]byte{
			[]byte(`{"type":"response.completed","response":{"id":"resp_ws_trace","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`),
		},
	}
	captureDialer := &openAIWSCaptureDialer{conn: captureConn}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)
	defer pool.Close()

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}

	account := &Account{
		ID:          503,
		Name:        "openai-ingress-trace",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-test",
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	serverErrCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{
			CompressionMode: coderws.CompressionContextTakeover,
		})
		if err != nil {
			serverErrCh <- err
			return
		}
		defer func() {
			_ = conn.CloseNow()
		}()

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		req := r.Clone(r.Context())
		req.Header = req.Header.Clone()
		req.Header.Set("User-Agent", "unit-test-agent/1.0")

		ctx, rootSpan := appelotel.GatewayTracer().Start(r.Context(), "gateway.responses_ws")
		defer rootSpan.End()
		ginCtx.Request = req.WithContext(ctx)

		readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		msgType, firstMessage, readErr := conn.Read(readCtx)
		cancel()
		if readErr != nil {
			serverErrCh <- readErr
			return
		}
		if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
			serverErrCh <- io.ErrUnexpectedEOF
			return
		}

		serverErrCh <- svc.ProxyResponsesWebSocketFromClient(ctx, ginCtx, conn, account, "sk-test", firstMessage, nil)
	}))
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() {
		_ = clientConn.CloseNow()
	}()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.1","stream":false}`))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	msgType, message, err := clientConn.Read(readCtx)
	cancelRead()
	require.NoError(t, err)
	require.Equal(t, coderws.MessageText, msgType)
	require.Contains(t, string(message), "resp_ws_trace")
	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))

	select {
	case serverErr := <-serverErrCh:
		require.NoError(t, serverErr)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for websocket proxy to exit")
	}

	root := findEndedSpanByName(t, recorder.Ended(), "gateway.responses_ws")
	attempt := findEndedSpanByName(t, recorder.Ended(), "gateway.upstream_attempt")
	require.Equal(t, root.SpanContext().SpanID(), attempt.Parent().SpanID())
	require.Equal(t, "responses_websockets_v2", spanAttributeValue(attempt, "robust2api.transport"))
}

func newSpanRecorderProvider() (*tracetest.SpanRecorder, *sdktrace.TracerProvider) {
	recorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider()
	traceProvider.RegisterSpanProcessor(recorder)
	otel.SetTracerProvider(traceProvider)
	return recorder, traceProvider
}

func findEndedSpanByName(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()

	for _, span := range spans {
		if span.Name() == name {
			return span
		}
	}
	t.Fatalf("span %q not found", name)
	return nil
}

func spanAttributeValue(span sdktrace.ReadOnlySpan, key string) string {
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			if value, ok := attr.Value.AsInterface().(string); ok {
				return value
			}
		}
	}
	return ""
}

func spanHasEvent(span sdktrace.ReadOnlySpan, name string) bool {
	for _, event := range span.Events() {
		if event.Name == name {
			return true
		}
	}
	return false
}
