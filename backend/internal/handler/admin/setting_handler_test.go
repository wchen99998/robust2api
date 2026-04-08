package admin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateEmbeddedOriginAllowedErrorMentionsEnvAndDeploymentSetting(t *testing.T) {
	t.Setenv(extraFrameSrcOriginsEnv, "https://allowed.example.com")

	err := validateEmbeddedOriginAllowed("https://blocked.example.com/dashboard", "Grafana URL")
	require.Error(t, err)
	require.Contains(t, err.Error(), extraFrameSrcOriginsEnv)
	require.Contains(t, err.Error(), "control.frontend.extraFrameSrcOrigins")
}
