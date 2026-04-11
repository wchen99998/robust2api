package accountops

import (
	"github.com/wchen99998/robust2api/internal/handler/admin"
	"github.com/wchen99998/robust2api/internal/service"
	"github.com/google/wire"
)

// ProviderSet groups account lifecycle, proxy, OAuth, and CRS/test flows.
var ProviderSet = wire.NewSet(
	service.NewAccountService,
	service.NewProxyService,
	service.NewOAuthService,
	service.NewOpenAIOAuthService,
	service.NewGeminiOAuthService,
	service.NewAntigravityOAuthService,
	service.NewCRSSyncService,
	service.NewAccountTestService,
	admin.NewAccountHandler,
	admin.NewOAuthHandler,
	admin.NewOpenAIOAuthHandler,
	admin.NewGeminiOAuthHandler,
	admin.NewAntigravityOAuthHandler,
	admin.NewProxyHandler,
)
