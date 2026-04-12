package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// AuthHandler serves the control-plane browser/BFF authentication surface.
type AuthHandler struct {
	cfg                       *config.Config
	controlSessionAuth        service.ControlSessionAuthService
	controlLocalCredentials   service.ControlLocalCredentialService
	controlRegistration       service.ControlRegistrationService
	controlProfile            service.ControlProfileService
	controlLocalMFA           service.ControlLocalMFAService
	externalIdentityProviders service.ExternalIdentityProviderRegistry
	userService               *service.UserService
	settingSvc                *service.SettingService
	version                   string
}

// NewAuthHandler creates the control-plane auth handler.
func NewAuthHandler(
	cfg *config.Config,
	controlSessionAuth service.ControlSessionAuthService,
	controlLocalCredentials service.ControlLocalCredentialService,
	controlRegistration service.ControlRegistrationService,
	controlProfile service.ControlProfileService,
	controlLocalMFA service.ControlLocalMFAService,
	userService *service.UserService,
	settingService *service.SettingService,
	buildInfo service.BuildInfo,
) *AuthHandler {
	return &AuthHandler{
		cfg:                     cfg,
		controlSessionAuth:      controlSessionAuth,
		controlLocalCredentials: controlLocalCredentials,
		controlRegistration:     controlRegistration,
		controlProfile:          controlProfile,
		controlLocalMFA:         controlLocalMFA,
		externalIdentityProviders: newControlExternalIdentityProviderRegistry(
			cfg,
			settingService,
			controlSessionAuth,
			controlRegistration,
		),
		userService: userService,
		settingSvc:  settingService,
		version:     buildInfo.Version,
	}
}
