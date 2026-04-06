//go:build unit

package bootstrap

import (
	"strings"
	"testing"
)

func TestValidateBootstrapEnv_AllRequired(t *testing.T) {
	env := BootstrapEnv{
		DatabaseHost:      "localhost",
		DatabasePort:      "5432",
		DatabaseUser:      "sub2api",
		DatabasePassword:  "secret",
		DatabaseDBName:    "sub2api",
		DatabaseSSLMode:   "disable",
		JWTSecret:         "abcdefghijklmnopqrstuvwxyz123456",
		TOTPEncryptionKey: strings.Repeat("a", 64),
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateBootstrapEnv_MissingJWTSecret(t *testing.T) {
	env := BootstrapEnv{
		DatabaseHost:      "localhost",
		DatabasePort:      "5432",
		DatabaseUser:      "sub2api",
		DatabasePassword:  "secret",
		DatabaseDBName:    "sub2api",
		DatabaseSSLMode:   "disable",
		TOTPEncryptionKey: strings.Repeat("a", 64),
	}
	err := env.Validate()
	if err == nil {
		t.Fatal("expected error for missing JWT_SECRET")
	}
}

func TestValidateBootstrapEnv_MissingTOTPKey(t *testing.T) {
	env := BootstrapEnv{
		DatabaseHost:     "localhost",
		DatabasePort:     "5432",
		DatabaseUser:     "sub2api",
		DatabasePassword: "secret",
		DatabaseDBName:   "sub2api",
		DatabaseSSLMode:  "disable",
		JWTSecret:        "abcdefghijklmnopqrstuvwxyz123456",
	}
	err := env.Validate()
	if err == nil {
		t.Fatal("expected error for missing TOTP_ENCRYPTION_KEY")
	}
}

func TestValidateBootstrapEnv_AdminEmailWithoutPassword(t *testing.T) {
	env := BootstrapEnv{
		DatabaseHost:      "localhost",
		DatabasePort:      "5432",
		DatabaseUser:      "sub2api",
		DatabasePassword:  "secret",
		DatabaseDBName:    "sub2api",
		DatabaseSSLMode:   "disable",
		JWTSecret:         "abcdefghijklmnopqrstuvwxyz123456",
		TOTPEncryptionKey: strings.Repeat("a", 64),
		AdminEmail:        "admin@example.com",
	}
	err := env.Validate()
	if err == nil {
		t.Fatal("expected error when ADMIN_EMAIL set without ADMIN_PASSWORD")
	}
}

func TestValidateBootstrapEnv_AdminEmailAndPassword(t *testing.T) {
	env := BootstrapEnv{
		DatabaseHost:      "localhost",
		DatabasePort:      "5432",
		DatabaseUser:      "sub2api",
		DatabasePassword:  "secret",
		DatabaseDBName:    "sub2api",
		DatabaseSSLMode:   "disable",
		JWTSecret:         "abcdefghijklmnopqrstuvwxyz123456",
		TOTPEncryptionKey: strings.Repeat("a", 64),
		AdminEmail:        "admin@example.com",
		AdminPassword:     "securepassword123",
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestLoadBootstrapEnvFromEnv(t *testing.T) {
	t.Setenv("DATABASE_HOST", "db.example.com")
	t.Setenv("DATABASE_PORT", "5432")
	t.Setenv("DATABASE_USER", "sub2api")
	t.Setenv("DATABASE_PASSWORD", "secret")
	t.Setenv("DATABASE_DBNAME", "sub2api")
	t.Setenv("DATABASE_SSLMODE", "require")
	t.Setenv("JWT_SECRET", "abcdefghijklmnopqrstuvwxyz123456")
	t.Setenv("TOTP_ENCRYPTION_KEY", strings.Repeat("a", 64))
	t.Setenv("ADMIN_EMAIL", "admin@test.com")
	t.Setenv("ADMIN_PASSWORD", "pass123")
	t.Setenv("RUN_MODE", "simple")

	env := LoadBootstrapEnv()
	if env.DatabaseHost != "db.example.com" {
		t.Errorf("expected db.example.com, got %s", env.DatabaseHost)
	}
	if env.AdminEmail != "admin@test.com" {
		t.Errorf("expected admin@test.com, got %s", env.AdminEmail)
	}
	if env.RunMode != "simple" {
		t.Errorf("expected simple, got %s", env.RunMode)
	}
}

func TestLoadBootstrapEnv_Defaults(t *testing.T) {
	for _, key := range []string{"DATABASE_HOST", "DATABASE_PORT", "DATABASE_USER", "DATABASE_PASSWORD",
		"DATABASE_DBNAME", "DATABASE_SSLMODE", "JWT_SECRET", "TOTP_ENCRYPTION_KEY",
		"ADMIN_EMAIL", "ADMIN_PASSWORD", "RUN_MODE"} {
		t.Setenv(key, "")
	}

	env := LoadBootstrapEnv()
	if env.DatabasePort != "5432" {
		t.Errorf("expected default port 5432, got %s", env.DatabasePort)
	}
	if env.DatabaseSSLMode != "disable" {
		t.Errorf("expected default sslmode disable, got %s", env.DatabaseSSLMode)
	}
}

func TestValidateBootstrapEnv_InvalidTOTPKey(t *testing.T) {
	env := BootstrapEnv{
		DatabaseHost:      "localhost",
		DatabasePort:      "5432",
		DatabaseUser:      "sub2api",
		DatabasePassword:  "secret",
		DatabaseDBName:    "sub2api",
		DatabaseSSLMode:   "disable",
		JWTSecret:         "abcdefghijklmnopqrstuvwxyz123456",
		TOTPEncryptionKey: "not-hex",
	}
	err := env.Validate()
	if err == nil {
		t.Fatal("expected error for invalid TOTP_ENCRYPTION_KEY")
	}
}
