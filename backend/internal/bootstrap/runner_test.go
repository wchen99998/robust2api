//go:build unit

package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/repository"
)

func TestRunBootstrap_ValidatesEnvFirst(t *testing.T) {
	env := BootstrapEnv{} // missing required fields
	err := Run(context.Background(), env)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRunBootstrap_FailsOnDBConnectionError(t *testing.T) {
	env := BootstrapEnv{
		DatabaseHost:      "invalid-host-that-does-not-exist",
		DatabasePort:      "5432",
		DatabaseUser:      "sub2api",
		DatabasePassword:  "secret",
		DatabaseDBName:    "sub2api",
		DatabaseSSLMode:   "disable",
		JWTSecret:         "abcdefghijklmnopqrstuvwxyz123456",
		TOTPEncryptionKey: strings.Repeat("a", 64),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := Run(ctx, env)
	if err == nil {
		t.Fatal("expected DB connection error")
	}
	if !strings.Contains(err.Error(), "ping database") && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected database connection failure, got: %v", err)
	}
}

func TestRunSteps_Order(t *testing.T) {
	order := []string{}
	steps := []step{
		{name: "step1", fn: func() error { order = append(order, "step1"); return nil }},
		{name: "step2", fn: func() error { order = append(order, "step2"); return nil }},
		{name: "step3", fn: func() error { order = append(order, "step3"); return nil }},
	}
	if err := runSteps(steps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 3 || order[0] != "step1" || order[1] != "step2" || order[2] != "step3" {
		t.Fatalf("unexpected order: %v", order)
	}
}

func TestRunSteps_StopsOnError(t *testing.T) {
	order := []string{}
	testErr := errors.New("step2 failed")
	steps := []step{
		{name: "step1", fn: func() error { order = append(order, "step1"); return nil }},
		{name: "step2", fn: func() error { order = append(order, "step2"); return testErr }},
		{name: "step3", fn: func() error { order = append(order, "step3"); return nil }},
	}
	err := runSteps(steps)
	if !errors.Is(err, testErr) {
		t.Fatalf("expected step2 error, got: %v", err)
	}
	if len(order) != 2 {
		t.Fatalf("step3 should not have run, order: %v", order)
	}
}

func TestAdminConcurrencyUsesRepositoryDefaults(t *testing.T) {
	if got := adminConcurrency(BootstrapEnv{RunMode: "simple"}); got != repository.SimpleModeTargetAdminConcurrency {
		t.Fatalf("simple mode concurrency = %d, want %d", got, repository.SimpleModeTargetAdminConcurrency)
	}
	if got := adminConcurrency(BootstrapEnv{}); got != repository.SimpleModeLegacyAdminConcurrency {
		t.Fatalf("standard mode concurrency = %d, want %d", got, repository.SimpleModeLegacyAdminConcurrency)
	}
}
