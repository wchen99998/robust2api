package logger

import "github.com/wchen99998/robust2api/internal/config"

func OptionsFromConfig(cfg config.LogConfig) InitOptions {
	return InitOptions{
		Level:           cfg.Level,
		Format:          cfg.Format,
		ServiceName:     cfg.ServiceName,
		Environment:     cfg.Environment,
		Caller:          cfg.Caller,
		StacktraceLevel: cfg.StacktraceLevel,
		Output: OutputOptions{
			ToStdout: cfg.Output.ToStdout,
		},
		Sampling: SamplingOptions{
			Enabled:    cfg.Sampling.Enabled,
			Initial:    cfg.Sampling.Initial,
			Thereafter: cfg.Sampling.Thereafter,
		},
	}
}
