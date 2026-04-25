package gateway

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/gateway/core"
	"github.com/Wei-Shaw/sub2api/internal/gateway/planning"
	"github.com/Wei-Shaw/sub2api/internal/gateway/provider"
	"github.com/Wei-Shaw/sub2api/internal/gateway/scheduler"
	"github.com/Wei-Shaw/sub2api/internal/gateway/transport"
	"github.com/Wei-Shaw/sub2api/internal/gateway/usage"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func NewCore(maxAccountSwitches int) core.GatewayCore {
	return core.NewEngine(
		planning.NewPlanner(maxAccountSwitches),
		provider.NewRegistry(
			provider.NewOpenAIAdapter(),
			provider.NewAnthropicAdapter(),
			provider.NewGeminiAdapter(),
			provider.NewAntigravityAdapter(),
		),
	)
}

func NewRuntimeCore(accountRepo service.AccountRepository, upstream service.HTTPUpstream, maxAccountSwitches int, maxResponseBytes int64) core.GatewayCore {
	return core.NewRuntimeEngine(
		planning.NewPlanner(maxAccountSwitches),
		provider.NewRegistry(
			provider.NewOpenAIAdapter(),
			provider.NewAnthropicAdapter(),
			provider.NewGeminiAdapter(),
			provider.NewAntigravityAdapter(),
		),
		scheduler.NewAccountSource(accountRepo),
		scheduler.NewSelector(),
		transport.NewHTTPExecutor(upstream, maxResponseBytes),
		transport.NewWebSocketExecutor(),
		usage.NewExtractor(),
	)
}

func ProvideRuntimeCore(accountRepo service.AccountRepository, upstream service.HTTPUpstream, cfg *config.Config) core.GatewayCore {
	maxSwitches := 3
	maxBytes := int64(16 << 20)
	if cfg != nil {
		if cfg.Gateway.MaxAccountSwitches > 0 {
			maxSwitches = cfg.Gateway.MaxAccountSwitches
		}
		if cfg.Gateway.MaxBodySize > 0 {
			maxBytes = cfg.Gateway.MaxBodySize
		}
	}
	return NewRuntimeCore(accountRepo, upstream, maxSwitches, maxBytes)
}
