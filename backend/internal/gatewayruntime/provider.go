package gatewayruntime

import (
	newgateway "github.com/Wei-Shaw/sub2api/internal/gateway"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/wire"
)

// ProviderSet owns the gateway request-path runtime graph.
var ProviderSet = wire.NewSet(
	service.APIProviderSet,
	newgateway.ProvideRuntimeCore,
	middleware.GatewayProviderSet,
	handler.GatewayProviderSet,
	server.GatewayProviderSet,
)
