package redis

import (
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	repository.ProvideRedis,
)
