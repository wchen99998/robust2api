package gatewayruntime

import (
	"github.com/wchen99998/robust2api/internal/handler"
	"github.com/wchen99998/robust2api/internal/server"
	"github.com/wchen99998/robust2api/internal/server/middleware"
	"github.com/wchen99998/robust2api/internal/service"
	"github.com/google/wire"
)

// ProviderSet owns the gateway request-path runtime graph.
var ProviderSet = wire.NewSet(
	service.APIProviderSet,
	middleware.GatewayProviderSet,
	handler.GatewayProviderSet,
	server.GatewayProviderSet,
)
