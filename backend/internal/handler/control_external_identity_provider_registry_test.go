//go:build unit

package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestControlExternalIdentityProviderRegistry_Providers_Auth0UsesOIDCDescriptor(t *testing.T) {
	registry := newControlExternalIdentityProviderRegistry(&config.Config{
		ControlAuth: config.ControlAuthConfig{
			Mode: service.ControlAuthModeExternalAuth0,
		},
		OIDC: config.OIDCConnectConfig{
			Enabled: true,
		},
	}, nil, &authBFFSessionAuthStub{}, &authBFFRegistrationStub{})

	providers := registry.Providers(context.Background())
	require.Len(t, providers, 1)
	require.Equal(t, "oidc", providers[0].ID)
	require.Equal(t, service.ControlAuthProviderTypeOIDC, providers[0].Type)
	require.Equal(t, "Auth0", providers[0].DisplayName)
	require.Equal(t, "/api/v1/oauth/oidc/start", providers[0].StartPath)
}

func TestControlExternalIdentityProviderRegistry_Providers_LocalModeIncludesEnabledProviders(t *testing.T) {
	registry := newControlExternalIdentityProviderRegistry(&config.Config{
		ControlAuth: config.ControlAuthConfig{
			Mode: service.ControlAuthModeLocal,
		},
		LinuxDo: config.LinuxDoConnectConfig{
			Enabled: true,
		},
		OIDC: config.OIDCConnectConfig{
			Enabled:      true,
			ProviderName: "Acme SSO",
		},
	}, nil, &authBFFSessionAuthStub{}, &authBFFRegistrationStub{})

	providers := registry.Providers(context.Background())
	require.Len(t, providers, 2)
	require.Equal(t, "linuxdo", providers[0].ID)
	require.Equal(t, "/api/v1/oauth/linuxdo/start", providers[0].StartPath)
	require.Equal(t, "oidc", providers[1].ID)
	require.Equal(t, "Acme SSO", providers[1].DisplayName)
}
