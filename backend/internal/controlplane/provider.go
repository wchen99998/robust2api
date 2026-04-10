package controlplane

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/wire"
)

// ProviderSet owns the control-plane HTTP orchestration graph.
var ProviderSet = wire.NewSet(
	service.APIProviderSet,
	middleware.ControlProviderSet,
	handler.ControlProviderSet,
	server.ControlProviderSet,
)
