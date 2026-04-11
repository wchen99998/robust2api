package service

import (
	"context"
	"testing"

	"github.com/wchen99998/robust2api/internal/config"
)

type hotPathSettingRepoStub struct {
	values           map[string]string
	getMultipleCalls int
}

func (s *hotPathSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *hotPathSettingRepoStub) GetValue(context.Context, string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *hotPathSettingRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *hotPathSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	s.getMultipleCalls++
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *hotPathSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *hotPathSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *hotPathSettingRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func TestSettingService_RefreshGatewayHotPathCacheUsesSnapshot(t *testing.T) {
	repo := &hotPathSettingRepoStub{
		values: map[string]string{
			SettingKeyEnableModelFallback:         "true",
			SettingKeyFallbackModelGemini:         "gemini-2.5-flash",
			SettingKeyEnableIdentityPatch:         "false",
			SettingKeyIdentityPatchPrompt:         "patched",
			SettingKeyAllowUngroupedKeyScheduling: "true",
		},
	}

	svc := NewSettingService(repo, &config.Config{})
	if err := svc.RefreshGatewayHotPathCache(context.Background()); err != nil {
		t.Fatalf("RefreshGatewayHotPathCache() error = %v", err)
	}
	if repo.getMultipleCalls != 1 {
		t.Fatalf("expected one GetMultiple call, got %d", repo.getMultipleCalls)
	}

	repo.values[SettingKeyEnableModelFallback] = "false"
	repo.values[SettingKeyFallbackModelGemini] = "gemini-2.5-pro"
	repo.values[SettingKeyEnableIdentityPatch] = "true"
	repo.values[SettingKeyIdentityPatchPrompt] = "new prompt"
	repo.values[SettingKeyAllowUngroupedKeyScheduling] = "false"

	if !svc.IsModelFallbackEnabled(context.Background()) {
		t.Fatal("expected model fallback to come from refreshed snapshot")
	}
	if got := svc.GetFallbackModel(context.Background(), PlatformGemini); got != "gemini-2.5-flash" {
		t.Fatalf("GetFallbackModel() = %q, want %q", got, "gemini-2.5-flash")
	}
	if svc.IsIdentityPatchEnabled(context.Background()) {
		t.Fatal("expected identity patch flag from snapshot")
	}
	if got := svc.GetIdentityPatchPrompt(context.Background()); got != "patched" {
		t.Fatalf("GetIdentityPatchPrompt() = %q, want %q", got, "patched")
	}
	if !svc.IsUngroupedKeySchedulingAllowed(context.Background()) {
		t.Fatal("expected ungrouped scheduling flag from snapshot")
	}
	if repo.getMultipleCalls != 1 {
		t.Fatalf("expected no additional GetMultiple calls after snapshot load, got %d", repo.getMultipleCalls)
	}
}
