//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestValidateServiceUpstreamBaseURLRequiresConfig(t *testing.T) {
	_, err := validateServiceUpstreamBaseURL(nil, "https://example.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "config is not available")
}

func TestValidateServiceUpstreamBaseURLDisabledRequiresHTTPS(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		},
	}

	_, err := validateServiceUpstreamBaseURL(cfg, "http://not-https.example.com")
	require.Error(t, err)

	normalized, err := validateServiceUpstreamBaseURL(cfg, "https://example.com")
	require.NoError(t, err)
	require.Equal(t, "https://example.com", normalized)
}

func TestValidateServiceUpstreamBaseURLDisabledAllowsHTTP(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowInsecureHTTP: true,
			},
		},
	}

	normalized, err := validateServiceUpstreamBaseURL(cfg, "http://not-https.example.com")
	require.NoError(t, err)
	require.Equal(t, "http://not-https.example.com", normalized)
}

func TestValidateServiceUpstreamBaseURLEnabledEnforcesAllowlist(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:       true,
				UpstreamHosts: []string{"example.com"},
			},
		},
	}

	_, err := validateServiceUpstreamBaseURL(cfg, "https://example.com")
	require.NoError(t, err)

	_, err = validateServiceUpstreamBaseURL(cfg, "https://evil.com")
	require.Error(t, err)
}
