package transport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
)

type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

type HTTPTransport struct {
	Client       Doer
	MaxBodyBytes int64
}

func (t HTTPTransport) RoundTrip(ctx context.Context, upstream *core.UpstreamRequest) (*core.UpstreamResult, error) {
	if upstream == nil {
		return nil, errors.New("upstream request is required")
	}
	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, upstream.Method, upstream.URL, bytes.NewReader(upstream.Body))
	if err != nil {
		return nil, err
	}
	req.Header = upstream.Headers.Clone()

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	limit := t.MaxBodyBytes
	if limit <= 0 {
		limit = 10 << 20
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("upstream response body too large: limit=%d", limit)
	}
	return &core.UpstreamResult{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header.Clone(),
		Body:       body,
		Duration:   time.Since(start),
	}, nil
}
