package transport

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
)

type Executor interface {
	Do(ctx context.Context, req *core.UpstreamRequest) (*core.UpstreamResult, error)
}
