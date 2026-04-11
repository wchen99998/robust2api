package health

import (
	apphealth "github.com/wchen99998/robust2api/internal/health"
	"github.com/google/wire"
)

type Checker = apphealth.Checker

var ProviderSet = wire.NewSet(
	apphealth.NewChecker,
)
