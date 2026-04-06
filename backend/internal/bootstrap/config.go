package bootstrap

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// BootstrapEnv holds all environment-based configuration for the bootstrap job.
type BootstrapEnv struct {
	DatabaseHost      string
	DatabasePort      string
	DatabaseUser      string
	DatabasePassword  string
	DatabaseDBName    string
	DatabaseSSLMode   string
	JWTSecret         string
	TOTPEncryptionKey string
	AdminEmail        string
	AdminPassword     string
	RunMode           string
	Timezone          string
}

// LoadBootstrapEnv reads bootstrap configuration from environment variables.
func LoadBootstrapEnv() BootstrapEnv {
	env := BootstrapEnv{
		DatabaseHost:      getenvOrDefault("DATABASE_HOST", ""),
		DatabasePort:      getenvOrDefault("DATABASE_PORT", "5432"),
		DatabaseUser:      getenvOrDefault("DATABASE_USER", ""),
		DatabasePassword:  os.Getenv("DATABASE_PASSWORD"),
		DatabaseDBName:    getenvOrDefault("DATABASE_DBNAME", ""),
		DatabaseSSLMode:   getenvOrDefault("DATABASE_SSLMODE", "disable"),
		JWTSecret:         os.Getenv("JWT_SECRET"),
		TOTPEncryptionKey: os.Getenv("TOTP_ENCRYPTION_KEY"),
		AdminEmail:        strings.TrimSpace(os.Getenv("ADMIN_EMAIL")),
		AdminPassword:     os.Getenv("ADMIN_PASSWORD"),
		RunMode:           strings.ToLower(strings.TrimSpace(os.Getenv("RUN_MODE"))),
		Timezone:          getenvOrDefault("TZ", "Asia/Shanghai"),
	}
	return env
}

// Validate checks that all required bootstrap inputs are present.
func (e *BootstrapEnv) Validate() error {
	if e.DatabaseHost == "" {
		return fmt.Errorf("DATABASE_HOST is required")
	}
	if e.DatabaseUser == "" {
		return fmt.Errorf("DATABASE_USER is required")
	}
	if e.DatabaseDBName == "" {
		return fmt.Errorf("DATABASE_DBNAME is required")
	}
	if strings.TrimSpace(e.JWTSecret) == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if len([]byte(strings.TrimSpace(e.JWTSecret))) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 bytes")
	}
	if strings.TrimSpace(e.TOTPEncryptionKey) == "" {
		return fmt.Errorf("TOTP_ENCRYPTION_KEY is required")
	}
	totpKeyBytes, err := hex.DecodeString(strings.TrimSpace(e.TOTPEncryptionKey))
	if err != nil {
		return fmt.Errorf("TOTP_ENCRYPTION_KEY must be valid hex: %w", err)
	}
	if len(totpKeyBytes) != 32 {
		return fmt.Errorf("TOTP_ENCRYPTION_KEY must be 32 bytes (64 hex chars)")
	}
	if e.AdminEmail != "" && strings.TrimSpace(e.AdminPassword) == "" {
		return fmt.Errorf("ADMIN_PASSWORD is required when ADMIN_EMAIL is set")
	}
	return nil
}

// DSN returns the PostgreSQL connection string.
func (e *BootstrapEnv) DSN() string {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		e.DatabaseHost, e.DatabasePort, e.DatabaseUser,
		e.DatabasePassword, e.DatabaseDBName, e.DatabaseSSLMode,
	)
	if e.Timezone != "" {
		dsn += fmt.Sprintf(" TimeZone=%s", e.Timezone)
	}
	return dsn
}

// IsSimpleMode returns true if RUN_MODE is "simple".
func (e *BootstrapEnv) IsSimpleMode() bool {
	return e.RunMode == "simple"
}

// WantsAdminSeed returns true if admin creation was requested.
func (e *BootstrapEnv) WantsAdminSeed() bool {
	return e.AdminEmail != ""
}

func getenvOrDefault(key, defaultVal string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultVal
}
