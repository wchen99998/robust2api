package maintenance

import (
	"github.com/wchen99998/robust2api/internal/service"
	"github.com/google/wire"
)

// ProviderSet owns the worker-side maintenance graph.
var ProviderSet = wire.NewSet(
	service.WorkerProviderSet,
)
