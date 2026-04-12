package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrInvalidCredentials     = infraerrors.Unauthorized("INVALID_CREDENTIALS", "invalid email or password")
	ErrUserNotActive          = infraerrors.Forbidden("USER_NOT_ACTIVE", "user is not active")
	ErrEmailExists            = infraerrors.Conflict("EMAIL_EXISTS", "email already exists")
	ErrEmailReserved          = infraerrors.BadRequest("EMAIL_RESERVED", "email is reserved")
	ErrInvalidToken           = infraerrors.Unauthorized("INVALID_TOKEN", "invalid token")
	ErrAccessTokenExpired     = infraerrors.Unauthorized("ACCESS_TOKEN_EXPIRED", "access token has expired")
	ErrTokenRevoked           = infraerrors.Unauthorized("TOKEN_REVOKED", "token has been revoked")
	ErrRefreshTokenInvalid    = infraerrors.Unauthorized("REFRESH_TOKEN_INVALID", "invalid refresh token")
	ErrRefreshTokenExpired    = infraerrors.Unauthorized("REFRESH_TOKEN_EXPIRED", "refresh token has expired")
	ErrRefreshTokenReused     = infraerrors.Unauthorized("REFRESH_TOKEN_REUSED", "refresh token has been reused")
	ErrEmailVerifyRequired    = infraerrors.BadRequest("EMAIL_VERIFY_REQUIRED", "email verification is required")
	ErrEmailSuffixNotAllowed  = infraerrors.BadRequest("EMAIL_SUFFIX_NOT_ALLOWED", "email suffix is not allowed")
	ErrRegDisabled            = infraerrors.Forbidden("REGISTRATION_DISABLED", "registration is currently disabled")
	ErrServiceUnavailable     = infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "service temporarily unavailable")
	ErrInvitationCodeRequired = infraerrors.BadRequest("INVITATION_CODE_REQUIRED", "invitation code is required")
	ErrInvitationCodeInvalid  = infraerrors.BadRequest("INVITATION_CODE_INVALID", "invalid or used invitation code")
)

type DefaultSubscriptionAssigner interface {
	AssignOrExtendSubscription(ctx context.Context, input *AssignSubscriptionInput) (*UserSubscription, bool, error)
}

func randomHexString(byteLength int) (string, error) {
	if byteLength <= 0 {
		byteLength = 16
	}
	buf := make([]byte, byteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func isReservedEmail(email string) bool {
	normalized := strings.ToLower(strings.TrimSpace(email))
	return strings.HasSuffix(normalized, LinuxDoConnectSyntheticEmailDomain)
}

func buildEmailSuffixNotAllowedError(whitelist []string) error {
	if len(whitelist) == 0 {
		return ErrEmailSuffixNotAllowed
	}

	allowed := strings.Join(whitelist, ", ")
	return infraerrors.BadRequest(
		"EMAIL_SUFFIX_NOT_ALLOWED",
		fmt.Sprintf("email suffix is not allowed, allowed suffixes: %s", allowed),
	).WithMetadata(map[string]string{
		"allowed_suffixes":     strings.Join(whitelist, ","),
		"allowed_suffix_count": strconv.Itoa(len(whitelist)),
	})
}
