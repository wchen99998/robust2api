//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type settingPublicRepoStub struct {
	values map[string]string
}

func (s *settingPublicRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *settingPublicRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingPublicRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingPublicRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *settingPublicRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingPublicRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingPublicRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestSettingService_GetPublicSettings_ExposesRegistrationEmailSuffixWhitelist(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyRegistrationEnabled:              "true",
			SettingKeyEmailVerifyEnabled:               "true",
			SettingKeyRegistrationEmailSuffixWhitelist: `["@EXAMPLE.com"," @foo.bar ","@invalid_domain",""]`,
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"@example.com", "@foo.bar"}, settings.RegistrationEmailSuffixWhitelist)
}

func TestSettingService_GetPublicSettings_FallsBackToConfiguredGrafanaURL(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyRegistrationEnabled: "true",
			SettingKeyGrafanaURL:          "",
		},
	}
	svc := NewSettingService(repo, &config.Config{
		GrafanaURL: "https://grafana.example.com",
	})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, "https://grafana.example.com", settings.GrafanaURL)
}

func TestSettingService_GetPublicSettings_PrefersDatabaseGrafanaURL(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyRegistrationEnabled: "true",
			SettingKeyGrafanaURL:          "https://grafana-db.example.com",
		},
	}
	svc := NewSettingService(repo, &config.Config{
		GrafanaURL: "https://grafana-config.example.com",
	})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, "https://grafana-db.example.com", settings.GrafanaURL)
}

func TestSettingService_GetPublicSettings_DoesNotTriggerOIDCDiscovery(t *testing.T) {
	var discoveryHits atomic.Int32
	discoveryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		discoveryHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorization_endpoint":"https://issuer.example.com/auth","token_endpoint":"https://issuer.example.com/token","userinfo_endpoint":"https://issuer.example.com/userinfo","jwks_uri":"https://issuer.example.com/jwks"}`))
	}))
	defer discoveryServer.Close()

	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyRegistrationEnabled: "true",
		},
	}
	svc := NewSettingService(repo, &config.Config{
		OIDC: config.OIDCConnectConfig{
			Enabled:             true,
			ProviderName:        "OIDC SSO",
			ClientID:            "client-id",
			ClientSecret:        "client-secret",
			IssuerURL:           discoveryServer.URL,
			DiscoveryURL:        discoveryServer.URL + "/.well-known/openid-configuration",
			RedirectURL:         "https://control.example.com/api/v1/auth/oauth/oidc/callback",
			FrontendRedirectURL: "/auth/oidc/callback",
			Scopes:              "openid email profile",
			TokenAuthMethod:     "client_secret_post",
			ValidateIDToken:     true,
			AllowedSigningAlgs:  "RS256",
			ClockSkewSeconds:    120,
		},
	})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.OIDCOAuthEnabled)
	require.Equal(t, "OIDC SSO", settings.OIDCOAuthProviderName)
	require.Zero(t, discoveryHits.Load())
}
