//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
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

type controlAuthCapabilityPromoRepoStub struct {
	byCode map[string]*PromoCode
}

func (s *controlAuthCapabilityPromoRepoStub) Create(context.Context, *PromoCode) error {
	return nil
}

func (s *controlAuthCapabilityPromoRepoStub) GetByID(context.Context, int64) (*PromoCode, error) {
	return nil, ErrPromoCodeNotFound
}

func (s *controlAuthCapabilityPromoRepoStub) GetByCode(_ context.Context, code string) (*PromoCode, error) {
	if promo, ok := s.byCode[code]; ok {
		return promo, nil
	}
	return nil, ErrPromoCodeNotFound
}

func (s *controlAuthCapabilityPromoRepoStub) GetByCodeForUpdate(ctx context.Context, code string) (*PromoCode, error) {
	return s.GetByCode(ctx, code)
}

func (s *controlAuthCapabilityPromoRepoStub) Update(context.Context, *PromoCode) error {
	return nil
}

func (s *controlAuthCapabilityPromoRepoStub) Delete(context.Context, int64) error {
	return nil
}

func (s *controlAuthCapabilityPromoRepoStub) List(context.Context, pagination.PaginationParams) ([]PromoCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (s *controlAuthCapabilityPromoRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string) ([]PromoCode, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (s *controlAuthCapabilityPromoRepoStub) CreateUsage(context.Context, *PromoCodeUsage) error {
	return nil
}

func (s *controlAuthCapabilityPromoRepoStub) GetUsageByPromoCodeAndUser(context.Context, int64, int64) (*PromoCodeUsage, error) {
	return nil, nil
}

func (s *controlAuthCapabilityPromoRepoStub) ListUsagesByPromoCode(context.Context, int64, pagination.PaginationParams) ([]PromoCodeUsage, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (s *controlAuthCapabilityPromoRepoStub) IncrementUsedCount(context.Context, int64) error {
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

func TestControlAuthService_RegistrationPreflight_UsesAuthModeAwareFlags(t *testing.T) {
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

	result, err := svc.RegistrationPreflight(context.Background(), "", "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.RegistrationEnabled)
	require.False(t, result.EmailVerificationRequired)
	require.Contains(t, result.Errors, "REGISTRATION_DISABLED")
}

func TestControlAuthService_RegistrationPreflight_ValidPromoIncludesBonusAmount(t *testing.T) {
	cfg := &config.Config{
		ControlAuth: config.ControlAuthConfig{
			Mode: ControlAuthModeLocal,
		},
	}
	settingSvc := NewSettingService(&controlAuthCapabilitySettingRepoStub{values: map[string]string{
		SettingKeyRegistrationEnabled: "true",
		SettingKeyPromoCodeEnabled:    "true",
		SettingKeyBackendModeEnabled:  "false",
	}}, cfg)
	svc := &ControlAuthService{
		cfg:            cfg,
		settingService: settingSvc,
		promoRepo: &controlAuthCapabilityPromoRepoStub{byCode: map[string]*PromoCode{
			"BONUS10": {
				Code:        "BONUS10",
				BonusAmount: 10,
				Status:      PromoCodeStatusActive,
			},
		}},
	}

	result, err := svc.RegistrationPreflight(context.Background(), "", "BONUS10", "")
	require.NoError(t, err)
	require.Equal(t, "valid", result.PromoStatus)
	require.NotNil(t, result.PromoBonusAmount)
	require.Equal(t, 10.0, *result.PromoBonusAmount)
	require.NotContains(t, result.Errors, "PROMO_CODE_INVALID")
}

func TestControlAuthService_RegistrationPreflight_PromoDisabledReturnsDisabled(t *testing.T) {
	cfg := &config.Config{
		ControlAuth: config.ControlAuthConfig{
			Mode: ControlAuthModeLocal,
		},
	}
	settingSvc := NewSettingService(&controlAuthCapabilitySettingRepoStub{values: map[string]string{
		SettingKeyRegistrationEnabled: "true",
		SettingKeyPromoCodeEnabled:    "false",
		SettingKeyBackendModeEnabled:  "false",
	}}, cfg)
	svc := &ControlAuthService{
		cfg:            cfg,
		settingService: settingSvc,
		promoRepo: &controlAuthCapabilityPromoRepoStub{byCode: map[string]*PromoCode{
			"BONUS10": {
				Code:        "BONUS10",
				BonusAmount: 10,
				Status:      PromoCodeStatusActive,
				ExpiresAt:   ptr(time.Now().Add(time.Hour)),
			},
		}},
	}

	result, err := svc.RegistrationPreflight(context.Background(), "", "BONUS10", "")
	require.NoError(t, err)
	require.Equal(t, "disabled", result.PromoStatus)
	require.Nil(t, result.PromoBonusAmount)
	require.Contains(t, result.Errors, "PROMO_CODE_DISABLED")
}
