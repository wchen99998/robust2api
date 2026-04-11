package ent

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/authfederatedidentity"
	"github.com/google/uuid"
)

func TestAuthFederatedIdentityAssignValuesRejectsNilID(t *testing.T) {
	t.Parallel()

	entity := &AuthFederatedIdentity{}
	err := entity.assignValues(
		[]string{authfederatedidentity.FieldID},
		[]any{&sql.NullInt64{}},
	)
	if err == nil {
		t.Fatal("expected nil ID scan to fail")
	}
	if !strings.Contains(err.Error(), "unexpected NULL value for field id") {
		t.Fatalf("expected NULL id error, got %v", err)
	}
}

func TestAuthFederatedIdentityAssignValuesReportsActualType(t *testing.T) {
	t.Parallel()

	entity := &AuthFederatedIdentity{}
	err := entity.assignValues(
		[]string{authfederatedidentity.FieldID},
		[]any{"bad"},
	)
	if err == nil {
		t.Fatal("expected type mismatch to fail")
	}
	if !strings.Contains(err.Error(), "unexpected type string for field id") {
		t.Fatalf("expected actual type in error, got %v", err)
	}
}

func TestAuthEntityStringsOmitNilOptionalFields(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, time.April, 11, 10, 11, 12, 0, time.UTC)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	subjectID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	sid := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	tests := []struct {
		name      string
		formatted string
		forbidden []string
	}{
		{
			name: "email verification",
			formatted: (&AuthEmailVerification{
				ID:        id,
				CreatedAt: ts,
				UpdatedAt: ts,
				Purpose:   "signup",
				Email:     "user@example.com",
				ExpiresAt: ts,
			}).String(),
			forbidden: []string{"subject_id=", "consumed_at=", ", ,", ", )"},
		},
		{
			name: "auth flow",
			formatted: (&AuthFlow{
				ID:         id,
				CreatedAt:  ts,
				UpdatedAt:  ts,
				Provider:   "google",
				Purpose:    "login",
				Issuer:     "issuer",
				RedirectTo: "/done",
				ExpiresAt:  ts,
			}).String(),
			forbidden: []string{"consumed_at=", ", ,", ", )"},
		},
		{
			name: "totp factor",
			formatted: (&AuthMFATOTPFactor{
				ID:        id,
				CreatedAt: ts,
				UpdatedAt: ts,
				Enabled:   true,
			}).String(),
			forbidden: []string{"enabled_at=", ", ,", ", )"},
		},
		{
			name: "password reset token",
			formatted: (&AuthPasswordResetToken{
				ID:        id,
				CreatedAt: ts,
				UpdatedAt: ts,
				SubjectID: subjectID,
				Email:     "user@example.com",
				ExpiresAt: ts,
			}).String(),
			forbidden: []string{"consumed_at=", ", ,", ", )"},
		},
		{
			name: "refresh token",
			formatted: (&AuthRefreshToken{
				ID:                "rtok",
				CreatedAt:         ts,
				UpdatedAt:         ts,
				Sid:               sid,
				SubjectID:         subjectID,
				LegacyUserID:      42,
				IdleExpiresAt:     ts,
				AbsoluteExpiresAt: ts,
			}).String(),
			forbidden: []string{"rotated_at=", "revoked_at=", ", ,", ", )"},
		},
		{
			name: "registration challenge",
			formatted: (&AuthRegistrationChallenge{
				ID:                id,
				CreatedAt:         ts,
				UpdatedAt:         ts,
				Provider:          "google",
				Issuer:            "issuer",
				ExternalSubject:   "external",
				Email:             "user@example.com",
				RegistrationEmail: "signup@example.com",
				Username:          "user",
				RedirectTo:        "/done",
				ExpiresAt:         ts,
			}).String(),
			forbidden: []string{"consumed_at=", ", ,", ", )"},
		},
		{
			name: "session",
			formatted: (&AuthSession{
				ID:                      sid,
				CreatedAt:               ts,
				UpdatedAt:               ts,
				SubjectID:               subjectID,
				LegacyUserID:            42,
				Status:                  "active",
				Amr:                     "pwd",
				LastSeenAt:              ts,
				ExpiresAt:               ts,
				AbsoluteExpiresAt:       ts,
				CurrentRefreshTokenHash: "hash",
				AuthVersion:             1,
			}).String(),
			forbidden: []string{"revoked_at=", ", ,", ", )"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, forbidden := range tt.forbidden {
				if strings.Contains(tt.formatted, forbidden) {
					t.Fatalf("did not expect %q in %s", forbidden, tt.formatted)
				}
			}
		})
	}
}
