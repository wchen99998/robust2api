package logging

import (
	platformconfig "github.com/Wei-Shaw/sub2api/internal/platform/config"
	applogger "github.com/Wei-Shaw/sub2api/internal/pkg/logger"
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
