package maintenance

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/wire"
)

// ProviderSet owns the worker-side maintenance graph.
var ProviderSet = wire.NewSet(
	service.WorkerProviderSet,
)
