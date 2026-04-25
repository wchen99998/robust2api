package anthropic

import (
	base "github.com/Wei-Shaw/sub2api/internal/gateway/provider"
)

func NewAdapter() *base.BasicAdapter {
	return base.NewAnthropicAdapter()
}
