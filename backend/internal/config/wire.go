package config

import "github.com/google/wire"

// GatewayProviderSet provides configuration for the gateway binary.
var GatewayProviderSet = wire.NewSet(
	ProvideGatewayConfig,
)

// ControlProviderSet provides configuration for the control binary.
var ControlProviderSet = wire.NewSet(
	ProvideControlConfig,
)

// WorkerProviderSet provides configuration for the worker binary.
var WorkerProviderSet = wire.NewSet(
	ProvideWorkerConfig,
)

// ProviderSet 提供配置层的依赖
var ProviderSet = wire.NewSet(
	ProvideConfig,
)

// ProvideConfig 提供应用配置
func ProvideConfig() (*Config, error) {
	return Load()
}

// ProvideGatewayConfig 提供 gateway 角色配置。
func ProvideGatewayConfig() (*Config, error) {
	return LoadGateway()
}

// ProvideControlConfig 提供 control 角色配置。
func ProvideControlConfig() (*Config, error) {
	return LoadControl()
}

// ProvideWorkerConfig 提供 worker 角色配置。
func ProvideWorkerConfig() (*Config, error) {
	return LoadWorker()
}
