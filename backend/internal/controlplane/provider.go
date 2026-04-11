package controlplane

import (
	"github.com/wchen99998/robust2api/internal/handler"
	"github.com/wchen99998/robust2api/internal/server"
	"github.com/wchen99998/robust2api/internal/server/middleware"
	"github.com/wchen99998/robust2api/internal/service"
	"github.com/google/wire"
)

// ProviderSet owns the control-plane HTTP orchestration graph.
var ProviderSet = wire.NewSet(
	service.APIProviderSet,
	middleware.ControlProviderSet,
	handler.ControlProviderSet,
	server.ControlProviderSet,
)
