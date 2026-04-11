package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

func (s *ControlAuthService) SyncUserIdentity(ctx context.Context, userID int64) (*IdentityBundle, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.authRepo.SyncLocalCredentialState(ctx, user)
}

func (s *ControlAuthService) RegistrationPreflight(ctx context.Context, email, promoCode, invitationCode string) (*RegistrationPreflightResult, error) {
	result := &RegistrationPreflightResult{
		RegistrationEnabled:       s.registrationEnabled(ctx),
		EmailVerificationRequired: s.emailVerificationEnabled(ctx),
		InvitationRequired:        s.settingService != nil && s.settingService.IsInvitationCodeEnabled(ctx),
		EmailSuffixAllowed:        true,
		PromoStatus:               "not_provided",
		InvitationStatus:          "not_required",
		Errors:                    []string{},
	}

	email = strings.TrimSpace(email)
	if !result.RegistrationEnabled {
		result.Errors = append(result.Errors, "REGISTRATION_DISABLED")
	}
	if email != "" {
		if isReservedEmail(email) {
			result.EmailSuffixAllowed = false
			result.Errors = append(result.Errors, "EMAIL_RESERVED")
		} else if err := s.validateRegistrationEmailPolicy(ctx, email); err != nil {
			result.EmailSuffixAllowed = false
			result.Errors = append(result.Errors, infraerrors.Reason(err))
		}
	}

	promoCode = strings.TrimSpace(promoCode)
	if promoCode != "" {
		promo, err := s.promoRepo.GetByCode(ctx, promoCode)
		switch {
		case err == nil && promo != nil && promo.Status == PromoCodeStatusActive && !promo.IsExpired() && (promo.MaxUses == 0 || promo.UsedCount < promo.MaxUses):
			result.PromoStatus = "valid"
		case err == nil && promo != nil && promo.IsExpired():
			result.PromoStatus = "expired"
			result.Errors = append(result.Errors, "PROMO_CODE_EXPIRED")
		case err == nil && promo != nil && promo.Status == PromoCodeStatusDisabled:
			result.PromoStatus = "disabled"
			result.Errors = append(result.Errors, "PROMO_CODE_DISABLED")
		case err == nil && promo != nil && promo.MaxUses > 0 && promo.UsedCount >= promo.MaxUses:
			result.PromoStatus = "overused"
			result.Errors = append(result.Errors, "PROMO_CODE_MAX_USED")
		default:
			result.PromoStatus = "invalid"
			result.Errors = append(result.Errors, "PROMO_CODE_INVALID")
		}
	}

	if result.InvitationRequired {
		result.InvitationStatus = "not_provided"
		invitationCode = strings.TrimSpace(invitationCode)
		if invitationCode == "" {
			result.Errors = append(result.Errors, "INVITATION_CODE_REQUIRED")
			return result, nil
		}

		redeemCode, err := s.redeemRepo.GetByCode(ctx, invitationCode)
		switch {
		case err == nil && redeemCode != nil && redeemCode.Type == RedeemTypeInvitation && redeemCode.Status == StatusUnused:
			result.InvitationStatus = "valid"
		case err == nil && redeemCode != nil && redeemCode.Status == StatusUsed:
			result.InvitationStatus = "used"
			result.Errors = append(result.Errors, "INVITATION_CODE_USED")
		default:
			result.InvitationStatus = "invalid"
			result.Errors = append(result.Errors, "INVITATION_CODE_INVALID")
		}
	}

	return result, nil
}

func (s *ControlAuthService) SendRegistrationEmailCode(ctx context.Context, email string) error {
	email = strings.TrimSpace(email)
	if !s.registrationEnabled(ctx) {
		return ErrRegDisabled
	}
	if !s.emailVerificationEnabled(ctx) {
		return ErrEmailVerificationOff
	}
	if isReservedEmail(email) {
		return ErrEmailReserved
	}
	if err := s.validateRegistrationEmailPolicy(ctx, email); err != nil {
		return err
	}
	exists, err := s.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		return ErrServiceUnavailable
	}
	if exists {
		return ErrEmailExists
	}

	code, err := s.emailService.GenerateVerifyCode()
	if err != nil {
		return fmt.Errorf("generate verification code: %w", err)
	}
	record := &EmailVerificationRecord{
		VerificationID: newUUIDString(),
		Purpose:        controlEmailPurposeRegistration,
		Email:          email,
		CodeHash:       hashToken(code),
		ExpiresAt:      time.Now().Add(controlEmailVerificationTTL),
	}
	if err := s.authRepo.CreateEmailVerification(ctx, record); err != nil {
		return fmt.Errorf("create email verification: %w", err)
	}
	return s.sendVerificationCodeEmail(ctx, email, code)
}

func (s *ControlAuthService) Register(ctx context.Context, input *ControlRegistrationInput) (*AuthenticatedIdentity, *ControlSessionTokens, error) {
	if input == nil {
		return nil, nil, infraerrors.BadRequest("INVALID_REQUEST", "registration payload is required")
	}
	if !s.registrationEnabled(ctx) {
		return nil, nil, ErrRegDisabled
	}

	email := strings.TrimSpace(input.Email)
	if isReservedEmail(email) {
		return nil, nil, ErrEmailReserved
	}
	if err := s.validateRegistrationEmailPolicy(ctx, email); err != nil {
		return nil, nil, err
	}
	if s.authService != nil {
		if err := s.authService.VerifyTurnstile(ctx, input.TurnstileToken, input.RemoteIP); err != nil {
			return nil, nil, err
		}
	}

	exists, err := s.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		return nil, nil, ErrServiceUnavailable
	}
	if exists {
		return nil, nil, ErrEmailExists
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin registration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)

	if s.settingService != nil && s.settingService.IsEmailVerifyEnabled(ctx) {
		if !s.emailVerificationEnabled(ctx) {
			return nil, nil, ErrEmailVerificationOff
		}
		if strings.TrimSpace(input.VerificationCode) == "" {
			return nil, nil, ErrEmailVerifyRequired
		}
		if _, err := s.authRepo.ConsumeEmailVerification(txCtx, controlEmailPurposeRegistration, email, hashToken(strings.TrimSpace(input.VerificationCode)), time.Now(), nil); err != nil {
			if errors.Is(err, ErrEmailVerificationNotFound) {
				return nil, nil, ErrInvalidVerifyCode
			}
			return nil, nil, err
		}
	}

	invitation, err := s.validateInvitationCode(txCtx, strings.TrimSpace(input.InvitationCode))
	if err != nil {
		return nil, nil, err
	}

	hashedPassword, err := hashPassword(input.Password)
	if err != nil {
		return nil, nil, fmt.Errorf("hash password: %w", err)
	}

	user := &User{
		Email:        email,
		PasswordHash: hashedPassword,
		Role:         RoleUser,
		Balance:      s.defaultUserBalance(ctx),
		Concurrency:  s.defaultUserConcurrency(ctx),
		Status:       StatusActive,
	}
	if err := s.userRepo.Create(txCtx, user); err != nil {
		if errors.Is(err, ErrEmailExists) {
			return nil, nil, ErrEmailExists
		}
		return nil, nil, fmt.Errorf("create user: %w", err)
	}

	bundle, err := s.authRepo.SyncLocalCredentialState(txCtx, user)
	if err != nil {
		return nil, nil, fmt.Errorf("sync local identity: %w", err)
	}
	if invitation != nil {
		if err := s.redeemRepo.Use(txCtx, invitation.ID, user.ID); err != nil {
			return nil, nil, fmt.Errorf("redeem invitation code: %w", err)
		}
	}
	if err := s.applyPromoCodeTx(txCtx, user, strings.TrimSpace(input.PromoCode)); err != nil {
		return nil, nil, err
	}
	if err := s.assignDefaultSubscriptionsTx(txCtx, user.ID); err != nil {
		return nil, nil, err
	}

	identity, tokens, snapshot, err := s.createSessionInTransaction(txCtx, bundle, user, "pwd")
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit registration transaction: %w", err)
	}
	s.tryStoreSessionSnapshot(ctx, snapshot)
	return identity, tokens, nil
}

func (s *ControlAuthService) RequestPasswordReset(ctx context.Context, email string) error {
	if !s.passwordResetEnabled(ctx) {
		return ErrPasswordResetDisabled
	}
	if s.emailService == nil {
		return ErrServiceUnavailable
	}

	email = strings.TrimSpace(email)
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil || user == nil || !user.IsActive() {
		return nil
	}

	bundle, err := s.authRepo.SyncLocalCredentialState(ctx, user)
	if err != nil {
		return err
	}
	token, err := s.emailService.GeneratePasswordResetToken()
	if err != nil {
		return fmt.Errorf("generate password reset token: %w", err)
	}
	record := &PasswordResetTokenRecord{
		ResetID:   newUUIDString(),
		SubjectID: bundle.Subject.SubjectID,
		Email:     email,
		TokenHash: hashToken(token),
		ExpiresAt: time.Now().Add(controlPasswordResetTTL),
	}
	if err := s.authRepo.CreatePasswordResetToken(ctx, record); err != nil {
		return fmt.Errorf("create password reset token: %w", err)
	}
	return s.sendPasswordResetEmail(ctx, email, token)
}

func (s *ControlAuthService) ResetPassword(ctx context.Context, email, token, newPassword string) error {
	if !s.passwordResetEnabled(ctx) {
		return ErrPasswordResetDisabled
	}

	hashedPassword, err := hashPassword(newPassword)
	if err != nil {
		return err
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin password reset transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	record, err := s.authRepo.ConsumePasswordResetToken(txCtx, strings.TrimSpace(email), hashToken(strings.TrimSpace(token)), time.Now())
	if err != nil {
		if errors.Is(err, ErrPasswordResetTokenNotFound) {
			return ErrInvalidResetToken
		}
		return err
	}

	user, err := s.getCurrentUserByEmail(txCtx, record.Email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return ErrInvalidResetToken
		}
		return err
	}
	if !user.IsActive() {
		return ErrUserNotActive
	}

	if err := s.updateUserPasswordHash(txCtx, user.ID, hashedPassword); err != nil {
		return err
	}
	user.PasswordHash = hashedPassword

	bundle, err := s.authRepo.SyncLocalCredentialState(txCtx, user)
	if err != nil {
		return err
	}
	if err := s.authRepo.RevokeAllSessions(txCtx, bundle.Subject.SubjectID, time.Now()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit password reset transaction: %w", err)
	}
	return nil
}

func (s *ControlAuthService) ChangePassword(ctx context.Context, identity *AuthenticatedIdentity, currentPassword, newPassword string) (*AuthenticatedIdentity, *ControlSessionTokens, error) {
	if identity == nil {
		return nil, nil, ErrInvalidToken
	}
	if !s.passwordChangeEnabled(ctx) {
		return nil, nil, ErrPasswordChangeDisabled
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin password change transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	user, err := s.getCurrentUserByID(txCtx, identity.LegacyUserID)
	if err != nil {
		return nil, nil, err
	}
	if !user.IsActive() {
		return nil, nil, ErrUserNotActive
	}
	if !checkPassword(currentPassword, user.PasswordHash) {
		return nil, nil, ErrPasswordIncorrect
	}

	hashedPassword, err := hashPassword(newPassword)
	if err != nil {
		return nil, nil, err
	}
	if err := s.updateUserPasswordHash(txCtx, user.ID, hashedPassword); err != nil {
		return nil, nil, err
	}
	user.PasswordHash = hashedPassword

	bundle, err := s.authRepo.SyncLocalCredentialState(txCtx, user)
	if err != nil {
		return nil, nil, err
	}
	if err := s.authRepo.RevokeAllSessions(txCtx, bundle.Subject.SubjectID, time.Now()); err != nil {
		return nil, nil, err
	}

	nextIdentity, tokens, snapshot, err := s.createSessionInTransaction(txCtx, bundle, user, identity.AMR)
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit password change transaction: %w", err)
	}
	_ = s.sessionCache.DeleteSessionSnapshot(ctx, identity.SessionID)
	s.tryStoreSessionSnapshot(ctx, snapshot)
	return nextIdentity, tokens, nil
}

func (s *ControlAuthService) UpdateProfile(ctx context.Context, identity *AuthenticatedIdentity, username *string) (*AuthenticatedIdentity, error) {
	if identity == nil {
		return nil, ErrInvalidToken
	}
	user, err := s.userRepo.GetByID(ctx, identity.LegacyUserID)
	if err != nil {
		return nil, err
	}
	if username != nil {
		user.Username = strings.TrimSpace(*username)
	}
	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}
	bundle, err := s.authRepo.EnsureSubjectAccount(ctx, user)
	if err != nil {
		return nil, err
	}
	return buildAuthenticatedIdentity(bundle, &SessionSnapshot{
		SessionID:         identity.SessionID,
		SubjectID:         identity.SubjectID,
		LegacyUserID:      identity.LegacyUserID,
		Status:            StatusActive,
		AMR:               identity.AMR,
		AuthVersion:       bundle.Subject.AuthVersion,
		ExpiresAt:         identity.SessionExpiresAt,
		AbsoluteExpiresAt: identity.SessionAbsoluteAt,
		LastSeenAt:        identity.SessionLastSeenAt,
	}, user), nil
}

func (s *ControlAuthService) CompleteExternalLogin(ctx context.Context, input *ControlExternalLoginRequest) (*ControlExternalLoginResult, error) {
	if input == nil {
		return nil, infraerrors.BadRequest("INVALID_REQUEST", "external login input is required")
	}
	if input.Identity == nil {
		return nil, infraerrors.BadRequest("INVALID_REQUEST", "external identity is required")
	}

	identityProfile := input.Identity
	provider := strings.TrimSpace(identityProfile.Provider)
	issuer := strings.TrimSpace(identityProfile.Issuer)
	externalSubject := strings.TrimSpace(identityProfile.Subject)
	if issuer == "" || externalSubject == "" {
		return nil, infraerrors.BadRequest("OAUTH_IDENTITY_INVALID", "external identity is incomplete")
	}

	bundle, err := s.authRepo.GetIdentityBundleByFederatedIdentity(ctx, provider, issuer, externalSubject)
	switch {
	case err == nil && bundle != nil && bundle.Subject != nil:
		user, err := s.getLinkedUserByID(ctx, bundle.Subject.LegacyUserID)
		if err != nil {
			return nil, err
		}
		if !user.IsActive() {
			return nil, ErrUserNotActive
		}
		if err := s.ensureBackendModeAdmin(ctx, user); err != nil {
			return nil, err
		}
		bundle, err = s.authRepo.SyncLocalCredentialState(ctx, user)
		if err != nil {
			return nil, err
		}
		identity, tokens, err := s.createAuthenticatedSession(ctx, bundle, user, input.AMR)
		if err != nil {
			return nil, err
		}
		return &ControlExternalLoginResult{Identity: identity, Tokens: tokens}, nil
	case err != nil && !errors.Is(err, ErrFederatedIdentityNotFound):
		return nil, err
	}

	loginEmail := strings.TrimSpace(identityProfile.LoginHint)

	if s.settingService != nil && s.settingService.IsBackendModeEnabled(ctx) {
		return nil, ErrRegDisabled
	}
	if registrationEmail := strings.TrimSpace(identityProfile.RegistrationEmail); registrationEmail != "" {
		if err := s.validateRegistrationEmailPolicy(ctx, registrationEmail); err != nil {
			return nil, err
		}
	}

	if s.settingService != nil && s.settingService.IsInvitationCodeEnabled(ctx) {
		challenge := &RegistrationChallengeRecord{
			ChallengeID:       newUUIDString(),
			Provider:          provider,
			Issuer:            issuer,
			ExternalSubject:   externalSubject,
			Email:             loginEmail,
			RegistrationEmail: strings.TrimSpace(identityProfile.RegistrationEmail),
			Username:          strings.TrimSpace(identityProfile.Username),
			RedirectTo:        strings.TrimSpace(input.RedirectTo),
			ExpiresAt:         time.Now().Add(controlRegistrationChallengeTTL),
		}
		if err := s.authRepo.CreateRegistrationChallenge(ctx, challenge); err != nil {
			return nil, err
		}
		return &ControlExternalLoginResult{Challenge: challenge}, nil
	}

	identity, tokens, err := s.registerOAuthSubject(ctx, &oauthRegistrationInput{
		Provider:          provider,
		Issuer:            issuer,
		ExternalSubject:   externalSubject,
		LoginEmail:        loginEmail,
		RegistrationEmail: strings.TrimSpace(identityProfile.RegistrationEmail),
		Username:          strings.TrimSpace(identityProfile.Username),
		AMR:               input.AMR,
	})
	if err != nil {
		return nil, err
	}
	return &ControlExternalLoginResult{Identity: identity, Tokens: tokens}, nil
}

func (s *ControlAuthService) CompleteOAuthRegistration(ctx context.Context, challengeID, invitationCode string) (*AuthenticatedIdentity, *ControlSessionTokens, error) {
	challenge, err := s.authRepo.GetRegistrationChallenge(ctx, strings.TrimSpace(challengeID))
	if err != nil {
		if errors.Is(err, ErrRegistrationChallengeNotFound) {
			return nil, nil, ErrRegistrationChallengeNotFound
		}
		return nil, nil, err
	}
	if challenge == nil || challenge.ConsumedAt != nil || time.Now().After(challenge.ExpiresAt) {
		return nil, nil, ErrRegistrationChallengeNotFound
	}

	return s.registerOAuthSubject(ctx, &oauthRegistrationInput{
		ChallengeID:       challenge.ChallengeID,
		Provider:          challenge.Provider,
		Issuer:            challenge.Issuer,
		ExternalSubject:   challenge.ExternalSubject,
		LoginEmail:        challenge.Email,
		RegistrationEmail: challenge.RegistrationEmail,
		Username:          challenge.Username,
		InvitationCode:    strings.TrimSpace(invitationCode),
		AMR:               challenge.Provider,
	})
}

type oauthRegistrationInput struct {
	ChallengeID       string
	Provider          string
	Issuer            string
	ExternalSubject   string
	LoginEmail        string
	RegistrationEmail string
	Username          string
	InvitationCode    string
	AMR               string
}

func (s *ControlAuthService) registerOAuthSubject(ctx context.Context, input *oauthRegistrationInput) (*AuthenticatedIdentity, *ControlSessionTokens, error) {
	randomPassword, err := randomHexString(32)
	if err != nil {
		return nil, nil, err
	}
	hashedPassword, err := hashPassword(randomPassword)
	if err != nil {
		return nil, nil, err
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	invitation, err := s.validateInvitationCode(txCtx, input.InvitationCode)
	if err != nil {
		return nil, nil, err
	}

	userEmail := strings.TrimSpace(input.RegistrationEmail)
	if userEmail == "" {
		userEmail = strings.TrimSpace(input.LoginEmail)
	}

	user := &User{
		Email:        userEmail,
		Username:     input.Username,
		PasswordHash: hashedPassword,
		Role:         RoleUser,
		Balance:      s.defaultUserBalance(ctx),
		Concurrency:  s.defaultUserConcurrency(ctx),
		Status:       StatusActive,
	}
	if err := s.userRepo.Create(txCtx, user); err != nil {
		if !errors.Is(err, ErrEmailExists) {
			return nil, nil, err
		}
		return nil, nil, ErrEmailExists
	}

	bundle, err := s.authRepo.EnsureSubjectAccount(txCtx, user)
	if err != nil {
		return nil, nil, err
	}
	if err := s.authRepo.LinkFederatedIdentity(txCtx, &FederatedIdentityRecord{
		SubjectID:       bundle.Subject.SubjectID,
		Provider:        input.Provider,
		Issuer:          input.Issuer,
		ExternalSubject: input.ExternalSubject,
		Email:           input.RegistrationEmail,
		Username:        input.Username,
	}); err != nil {
		return nil, nil, err
	}
	if invitation != nil {
		if err := s.redeemRepo.Use(txCtx, invitation.ID, user.ID); err != nil {
			return nil, nil, err
		}
	}
	if err := s.assignDefaultSubscriptionsTx(txCtx, user.ID); err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(input.ChallengeID) != "" {
		if _, err := s.authRepo.ConsumeRegistrationChallenge(txCtx, strings.TrimSpace(input.ChallengeID), time.Now()); err != nil {
			return nil, nil, err
		}
	}

	identity, tokens, snapshot, err := s.createSessionInTransaction(txCtx, bundle, user, input.AMR)
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	s.tryStoreSessionSnapshot(ctx, snapshot)
	return identity, tokens, nil
}

func (s *ControlAuthService) createSessionInTransaction(ctx context.Context, bundle *IdentityBundle, user *User, amr string) (*AuthenticatedIdentity, *ControlSessionTokens, *SessionSnapshot, error) {
	now := time.Now()
	sessionRecord, refreshRecord, rawRefreshToken, err := s.newSessionRecords(bundle, user, amr, now)
	if err != nil {
		return nil, nil, nil, err
	}
	accessToken, accessExpiry, err := s.signAccessToken(bundle.Subject.SubjectID, sessionRecord.SessionID, bundle.Subject.AuthVersion, amr, now)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := s.authRepo.CreateSession(ctx, sessionRecord, refreshRecord); err != nil {
		return nil, nil, nil, err
	}
	snapshot := sessionRecordToSnapshot(sessionRecord)
	return buildAuthenticatedIdentity(bundle, snapshot, user), &ControlSessionTokens{
		AccessToken:       accessToken,
		RefreshToken:      rawRefreshToken,
		AccessExpiresAt:   accessExpiry,
		IdleRefreshExpiry: refreshRecord.IdleExpiresAt,
		AbsoluteExpiry:    refreshRecord.AbsoluteExpiresAt,
	}, snapshot, nil
}

func (s *ControlAuthService) validateInvitationCode(ctx context.Context, invitationCode string) (*RedeemCode, error) {
	if s.settingService == nil || !s.settingService.IsInvitationCodeEnabled(ctx) {
		return nil, nil
	}
	if strings.TrimSpace(invitationCode) == "" {
		return nil, ErrInvitationCodeRequired
	}
	redeemCode, err := s.redeemRepo.GetByCode(ctx, invitationCode)
	if err != nil || redeemCode == nil || redeemCode.Type != RedeemTypeInvitation || redeemCode.Status != StatusUnused {
		return nil, ErrInvitationCodeInvalid
	}
	return redeemCode, nil
}

func (s *ControlAuthService) applyPromoCodeTx(ctx context.Context, user *User, promoCode string) error {
	if strings.TrimSpace(promoCode) == "" {
		return nil
	}
	if s.settingService == nil || !s.settingService.IsPromoCodeEnabled(ctx) {
		return ErrPromoCodeDisabled
	}
	promo, err := s.promoRepo.GetByCodeForUpdate(ctx, promoCode)
	if err != nil || promo == nil {
		return ErrPromoCodeInvalid
	}
	switch {
	case promo.IsExpired():
		return ErrPromoCodeExpired
	case promo.Status == PromoCodeStatusDisabled:
		return ErrPromoCodeDisabled
	case promo.MaxUses > 0 && promo.UsedCount >= promo.MaxUses:
		return ErrPromoCodeMaxUsed
	}

	if err := s.userRepo.UpdateBalance(ctx, user.ID, promo.BonusAmount); err != nil {
		return err
	}
	if err := s.promoRepo.CreateUsage(ctx, &PromoCodeUsage{
		PromoCodeID: promo.ID,
		UserID:      user.ID,
		BonusAmount: promo.BonusAmount,
		UsedAt:      time.Now(),
	}); err != nil {
		return err
	}
	if err := s.promoRepo.IncrementUsedCount(ctx, promo.ID); err != nil {
		return err
	}
	user.Balance += promo.BonusAmount
	return nil
}

func (s *ControlAuthService) assignDefaultSubscriptionsTx(ctx context.Context, userID int64) error {
	if s.settingService == nil || s.defaultSubAssigner == nil || userID <= 0 {
		return nil
	}
	for _, item := range s.settingService.GetDefaultSubscriptions(ctx) {
		if _, _, err := s.defaultSubAssigner.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
			UserID:       userID,
			GroupID:      item.GroupID,
			ValidityDays: item.ValidityDays,
			Notes:        "auto assigned by default user subscriptions setting",
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *ControlAuthService) ensureBackendModeAdmin(ctx context.Context, user *User) error {
	if s.settingService != nil && s.settingService.IsBackendModeEnabled(ctx) && (user == nil || !user.IsAdmin()) {
		return infraerrors.Forbidden("BACKEND_MODE_ONLY_ADMIN", "backend mode is active. only admin login is allowed")
	}
	return nil
}

func (s *ControlAuthService) defaultUserBalance(ctx context.Context) float64 {
	if s.settingService != nil {
		return s.settingService.GetDefaultBalance(ctx)
	}
	if s.cfg != nil {
		return s.cfg.Default.UserBalance
	}
	return 0
}

func (s *ControlAuthService) defaultUserConcurrency(ctx context.Context) int {
	if s.settingService != nil {
		return s.settingService.GetDefaultConcurrency(ctx)
	}
	if s.cfg != nil {
		return s.cfg.Default.UserConcurrency
	}
	return 1
}

func (s *ControlAuthService) isRegistrationAllowed(ctx context.Context) bool {
	if s.settingService == nil {
		return false
	}
	if s.settingService.IsBackendModeEnabled(ctx) {
		return false
	}
	return s.settingService.IsRegistrationEnabled(ctx)
}

func (s *ControlAuthService) validateRegistrationEmailPolicy(ctx context.Context, email string) error {
	if s.settingService == nil {
		return nil
	}
	whitelist := s.settingService.GetRegistrationEmailSuffixWhitelist(ctx)
	if !IsRegistrationEmailSuffixAllowed(email, whitelist) {
		return buildEmailSuffixNotAllowedError(whitelist)
	}
	return nil
}

func (s *ControlAuthService) sendVerificationCodeEmail(ctx context.Context, email, code string) error {
	if s.emailService == nil {
		return ErrServiceUnavailable
	}
	siteName := "robust2api"
	if s.settingService != nil {
		siteName = s.settingService.GetSiteName(ctx)
	}
	subject := fmt.Sprintf("[%s] Email Verification Code", siteName)
	body := s.emailService.buildVerifyCodeEmailBody(code, siteName)
	return s.emailService.SendEmail(ctx, email, subject, body)
}

func (s *ControlAuthService) sendPasswordResetEmail(ctx context.Context, email, token string) error {
	if s.emailService == nil {
		return ErrServiceUnavailable
	}
	baseURL := s.frontendBaseURL(ctx)
	if baseURL == "" {
		return ErrServiceUnavailable
	}
	siteName := "robust2api"
	if s.settingService != nil {
		siteName = s.settingService.GetSiteName(ctx)
	}
	resetQuery := url.Values{
		"email": []string{email},
		"token": []string{token},
	}
	resetURL := fmt.Sprintf("%s/reset-password?%s", strings.TrimSuffix(baseURL, "/"), resetQuery.Encode())
	subject := fmt.Sprintf("[%s] 密码重置请求", siteName)
	body := s.emailService.buildPasswordResetEmailBody(resetURL, siteName)
	return s.emailService.SendEmail(ctx, email, subject, body)
}

func hashPassword(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", infraerrors.BadRequest("PASSWORD_REQUIRED", "password is required")
	}
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
