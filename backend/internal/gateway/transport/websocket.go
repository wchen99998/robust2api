package transport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	coderws "github.com/coder/websocket"
)

const webSocketReadLimitBytes int64 = 16 * 1024 * 1024

type WebSocketExecutor struct{}

func NewWebSocketExecutor() *WebSocketExecutor {
	return &WebSocketExecutor{}
}

func (e *WebSocketExecutor) Proxy(ctx context.Context, req *core.UpstreamRequest, client core.WebSocketConn, firstType core.WebSocketMessageType, firstPayload []byte) error {
	if req == nil {
		return errors.New("upstream request is required")
	}
	if client == nil {
		return errors.New("websocket client is required")
	}
	targetURL, err := URLWithWebSocketScheme(req.URL)
	if err != nil {
		return err
	}
	opts := &coderws.DialOptions{
		HTTPHeader:      cloneHeader(req.Headers),
		CompressionMode: coderws.CompressionContextTakeover,
	}
	if proxy := strings.TrimSpace(req.ProxyURL); proxy != "" {
		proxyClient, err := proxyHTTPClient(proxy)
		if err != nil {
			return err
		}
		opts.HTTPClient = proxyClient
	}
	upstream, _, err := coderws.Dial(ctx, targetURL, opts)
	if err != nil {
		return err
	}
	defer func() {
		_ = upstream.CloseNow()
	}()
	upstream.SetReadLimit(webSocketReadLimitBytes)
	if len(firstPayload) > 0 {
		if err := upstream.Write(ctx, toCoderMessageType(firstType), firstPayload); err != nil {
			return err
		}
	}
	proxyCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		errCh <- copyCoderToClient(proxyCtx, upstream, client)
		cancel()
	}()
	go func() {
		defer wg.Done()
		errCh <- copyClientToCoder(proxyCtx, client, upstream)
		cancel()
	}()
	wg.Wait()
	close(errCh)
	for proxyErr := range errCh {
		if proxyErr != nil && !isExpectedWebSocketClose(proxyErr) {
			return proxyErr
		}
	}
	_ = upstream.Close(coderws.StatusNormalClosure, "")
	_ = client.Close(int(coderws.StatusNormalClosure), "")
	return nil
}

func URLWithWebSocketScheme(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid upstream websocket url: %w", err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	case "wss", "ws":
	default:
		return "", fmt.Errorf("unsupported websocket scheme: %s", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", errors.New("upstream websocket host is empty")
	}
	return parsed.String(), nil
}

func copyCoderToClient(ctx context.Context, upstream *coderws.Conn, client core.WebSocketConn) error {
	for {
		typ, payload, err := upstream.Read(ctx)
		if err != nil {
			return err
		}
		if err := client.Write(ctx, fromCoderMessageType(typ), payload); err != nil {
			return err
		}
	}
}

func copyClientToCoder(ctx context.Context, client core.WebSocketConn, upstream *coderws.Conn) error {
	for {
		typ, payload, err := client.Read(ctx)
		if err != nil {
			return err
		}
		if err := upstream.Write(ctx, toCoderMessageType(typ), payload); err != nil {
			return err
		}
	}
}

func toCoderMessageType(typ core.WebSocketMessageType) coderws.MessageType {
	if typ == core.WebSocketMessageBinary {
		return coderws.MessageBinary
	}
	return coderws.MessageText
}

func fromCoderMessageType(typ coderws.MessageType) core.WebSocketMessageType {
	if typ == coderws.MessageBinary {
		return core.WebSocketMessageBinary
	}
	return core.WebSocketMessageText
}

func proxyHTTPClient(rawProxy string) (*http.Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawProxy))
	if err != nil {
		return nil, fmt.Errorf("invalid proxy url: %w", err)
	}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:               http.ProxyURL(parsed),
			MaxIdleConns:        128,
			MaxIdleConnsPerHost: 64,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			ForceAttemptHTTP2:   true,
		},
	}, nil
}

func isExpectedWebSocketClose(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return true
	}
	switch coderws.CloseStatus(err) {
	case coderws.StatusNormalClosure, coderws.StatusGoingAway:
		return true
	default:
		return false
	}
}

func cloneHeader(headers http.Header) http.Header {
	if len(headers) == 0 {
		return nil
	}
	return headers.Clone()
}
