package otel

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestInit_Disabled(t *testing.T) {
	cfg := &config.OtelConfig{Enabled: false}
	provider, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if provider == nil {
		t.Fatal("Init() returned nil provider")
	}
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestInit_Enabled_NoEndpoint(t *testing.T) {
	cfg := &config.OtelConfig{
		Enabled:         true,
		ServiceName:     "test-service",
		Endpoint:        "http://localhost:4318",
		TraceSampleRate: 1.0,
		MetricsPort:     0,
	}
	provider, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if provider == nil {
		t.Fatal("Init() returned nil provider")
	}
	if provider.TracerProvider() == nil {
		t.Fatal("TracerProvider() returned nil")
	}
	if provider.MeterProvider() == nil {
		t.Fatal("MeterProvider() returned nil")
	}
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}
