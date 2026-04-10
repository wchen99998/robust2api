package otel

import (
	appotel "github.com/Wei-Shaw/sub2api/internal/pkg/otel"
	"github.com/google/wire"
)

type Provider = appotel.Provider
type MetricsServer = appotel.MetricsServer

var ProviderSet = wire.NewSet(
	appotel.ProviderSet,
)
