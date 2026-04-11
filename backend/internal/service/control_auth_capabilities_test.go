//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type controlAuthCapabilitySettingRepoStub struct {
	values map[string]string
}

func (s *controlAuthCapabilitySettingRepoStub) Get(_ context.Context, key string) (*Setting, error) {
	if value, ok := s.values[key]; ok {
		return &Setting{Key: key, Value: value}, nil
	}
	return nil, ErrSettingNotFound
}

func (s *controlAuthCapabilitySettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (s *controlAuthCapabilitySettingRepoStub) Set(_ context.Context, key, value string) error {
	s.values[key] = value
	return nil
}

func (s *controlAuthCapabilitySettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *controlAuthCapabilitySettingRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *controlAuthCapabilitySettingRepoStub) GetAll(_ context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *controlAuthCapabilitySettingRepoStub) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
}

func TestControlAuthService_AuthCapabilities_LocalMode(t *testing.T) {
	cfg := &config.Config{
		ControlAuth: config.ControlAuthConfig{
			Mode: ControlAuthModeLocal,
		},
	}
	settingSvc := NewSettingService(&controlAuthCapabilitySettingRepoStub{values: map[string]string{
		SettingKeyRegistrationEnabled:  "true",
		SettingKeyEmailVerifyEnabled:   "true",
		SettingKeyPasswordResetEnabled: "true",
		SettingKeyTotpEnabled:          "true",
		SettingKeyBackendModeEnabled:   "false",
	}}, cfg)
	svc := &ControlAuthService{
		cfg:            cfg,
		settingService: settingSvc,
	}

	caps := svc.AuthCapabilities(context.Background())
	require.Equal(t, ControlAuthModeLocal, caps.Provider)
	require.True(t, caps.PasswordLoginEnabled)
	require.True(t, caps.RegistrationEnabled)
	require.True(t, caps.EmailVerificationEnabled)
	require.True(t, caps.PasswordResetEnabled)
	require.True(t, caps.PasswordChangeEnabled)
	require.True(t, caps.MFASelfServiceEnabled)
	require.True(t, caps.ProfileSelfServiceEnabled)
}

func TestControlAuthService_AuthCapabilities_ExternalOIDCMode(t *testing.T) {
	cfg := &config.Config{
		ControlAuth: config.ControlAuthConfig{
			Mode: ControlAuthModeExternalOIDC,
		},
	}
	settingSvc := NewSettingService(&controlAuthCapabilitySettingRepoStub{values: map[string]string{
		SettingKeyRegistrationEnabled:  "true",
		SettingKeyEmailVerifyEnabled:   "true",
		SettingKeyPasswordResetEnabled: "true",
		SettingKeyTotpEnabled:          "true",
		SettingKeyBackendModeEnabled:   "false",
	}}, cfg)
	svc := &ControlAuthService{
		cfg:            cfg,
		settingService: settingSvc,
	}

	caps := svc.AuthCapabilities(context.Background())
	require.Equal(t, ControlAuthModeExternalOIDC, caps.Provider)
	require.False(t, caps.PasswordLoginEnabled)
	require.False(t, caps.RegistrationEnabled)
	require.False(t, caps.EmailVerificationEnabled)
	require.False(t, caps.PasswordResetEnabled)
	require.False(t, caps.PasswordChangeEnabled)
	require.False(t, caps.MFASelfServiceEnabled)
	require.True(t, caps.ProfileSelfServiceEnabled)
}
