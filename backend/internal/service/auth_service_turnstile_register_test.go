//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type turnstileVerifierSpy struct {
	called    int
	lastToken string
	result    *TurnstileVerifyResponse
	err       error
}

func (s *turnstileVerifierSpy) VerifyToken(_ context.Context, _ string, token, _ string) (*TurnstileVerifyResponse, error) {
	s.called++
	s.lastToken = token
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &TurnstileVerifyResponse{Success: true}, nil
}

func newControlAuthServiceForTurnstileTest(settings map[string]string, verifier TurnstileVerifier) *ControlAuthService {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Mode: "release",
		},
		Turnstile: config.TurnstileConfig{
			Required: true,
		},
	}

	settingService := NewSettingService(&settingRepoStub{values: settings}, cfg)
	turnstileService := NewTurnstileService(settingService, verifier)

	return &ControlAuthService{
		cfg:              cfg,
		settingService:   settingService,
		turnstileService: turnstileService,
	}
}

func TestControlAuthService_VerifyTurnstile_EnforcesTokenEvenWhenEmailVerificationCodeProvided(t *testing.T) {
	verifier := &turnstileVerifierSpy{}
	service := newControlAuthServiceForTurnstileTest(map[string]string{
		SettingKeyEmailVerifyEnabled:  "true",
		SettingKeyTurnstileEnabled:    "true",
		SettingKeyTurnstileSecretKey:  "secret",
		SettingKeyRegistrationEnabled: "true",
	}, verifier)

	err := service.verifyTurnstile(context.Background(), "", "127.0.0.1")
	require.ErrorIs(t, err, ErrTurnstileVerificationFailed)
	require.Equal(t, 0, verifier.called)
}

func TestControlAuthService_VerifyTurnstile_RequiresTokenWhenEnabled(t *testing.T) {
	verifier := &turnstileVerifierSpy{}
	service := newControlAuthServiceForTurnstileTest(map[string]string{
		SettingKeyEmailVerifyEnabled: "true",
		SettingKeyTurnstileEnabled:   "true",
		SettingKeyTurnstileSecretKey: "secret",
	}, verifier)

	err := service.verifyTurnstile(context.Background(), "", "127.0.0.1")
	require.ErrorIs(t, err, ErrTurnstileVerificationFailed)
}

func TestControlAuthService_VerifyTurnstile_UsesTurnstileWhenEnabled(t *testing.T) {
	verifier := &turnstileVerifierSpy{}
	service := newControlAuthServiceForTurnstileTest(map[string]string{
		SettingKeyEmailVerifyEnabled: "false",
		SettingKeyTurnstileEnabled:   "true",
		SettingKeyTurnstileSecretKey: "secret",
	}, verifier)

	err := service.verifyTurnstile(context.Background(), "turnstile-token", "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, 1, verifier.called)
	require.Equal(t, "turnstile-token", verifier.lastToken)
}

func TestControlAuthService_VerifyTurnstile_ReturnsNotConfiguredWhenEnabledWithoutService(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Mode: "debug"},
		Turnstile: config.TurnstileConfig{
			Required: false,
		},
	}

	service := &ControlAuthService{
		cfg: cfg,
		settingService: NewSettingService(&settingRepoStub{values: map[string]string{
			SettingKeyTurnstileEnabled:   "true",
			SettingKeyTurnstileSecretKey: "secret",
		}}, cfg),
	}

	err := service.verifyTurnstile(context.Background(), "turnstile-token", "127.0.0.1")
	require.ErrorIs(t, err, ErrTurnstileNotConfigured)
}
