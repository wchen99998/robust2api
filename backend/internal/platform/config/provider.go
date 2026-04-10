package config

import (
	appconfig "github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/wire"
)

type Config = appconfig.Config
type LogConfig = appconfig.LogConfig

var GatewayProviderSet = wire.NewSet(
	appconfig.ProvideGatewayConfig,
)

var ControlProviderSet = wire.NewSet(
	appconfig.ProvideControlConfig,
)

var WorkerProviderSet = wire.NewSet(
	appconfig.ProvideWorkerConfig,
)

var BillingProviderSet = wire.NewSet(
	appconfig.ProvideBillingConfig,
)

func LoadGateway() (*Config, error) {
	return appconfig.LoadGateway()
}

func LoadControl() (*Config, error) {
	return appconfig.LoadControl()
}

func LoadWorker() (*Config, error) {
	return appconfig.LoadWorker()
}

func LoadBilling() (*Config, error) {
	return appconfig.LoadBilling()
}
