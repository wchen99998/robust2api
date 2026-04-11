package redis

import (
	"github.com/wchen99998/robust2api/internal/repository"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	repository.ProvideRedis,
)
