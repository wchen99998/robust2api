package logging

import (
	platformconfig "github.com/wchen99998/robust2api/internal/platform/config"
	applogger "github.com/wchen99998/robust2api/internal/pkg/logger"
)

func InitBootstrap() {
	applogger.InitBootstrap()
}

func Init(cfg platformconfig.LogConfig) error {
	return applogger.Init(applogger.OptionsFromConfig(cfg))
}

func Sync() {
	applogger.Sync()
}
