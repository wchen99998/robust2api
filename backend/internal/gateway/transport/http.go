package transport

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type HTTPExecutor struct {
	upstream service.HTTPUpstream
	maxBytes int64
}

func NewHTTPExecutor(upstream service.HTTPUpstream, maxBytes int64) *HTTPExecutor {
	if maxBytes <= 0 {
		maxBytes = 16 << 20
	}
	return &HTTPExecutor{upstream: upstream, maxBytes: maxBytes}
}

func (e *HTTPExecutor) Do(ctx context.Context, req *core.UpstreamRequest) (*core.UpstreamResult, error) {
	start := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	httpReq.Header = req.Headers.Clone()
	resp, err := e.upstream.Do(httpReq, req.ProxyURL, req.AccountID, req.Concurrency)
	if err != nil {
		return &core.UpstreamResult{UpstreamError: err, Duration: time.Since(start)}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, e.maxBytes+1))
	if readErr != nil {
		return nil, readErr
	}
	if int64(len(body)) > e.maxBytes {
		body = body[:e.maxBytes]
	}
	return &core.UpstreamResult{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header.Clone(),
		Body:       body,
		Duration:   time.Since(start),
	}, nil
}
