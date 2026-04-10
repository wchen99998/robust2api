package health

import (
	apphealth "github.com/Wei-Shaw/sub2api/internal/health"
	"github.com/google/wire"
)

type Checker = apphealth.Checker

var ProviderSet = wire.NewSet(
	apphealth.NewChecker,
)
