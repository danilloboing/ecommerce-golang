package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/email"
	"github.com/danilloboing/marketplace-golang/internal/platform/passwords"
	"github.com/danilloboing/marketplace-golang/internal/platform/tokens"
	"github.com/google/uuid"
)

// IdentityServiceDeps lists every collaborator. All required.
type IdentityServiceDeps struct {
	Users        UserRepository
	AuthMethods  AuthMethodRepository
	VerifyTokens EmailVerifyTokenRepository
	ResetTokens  PasswordResetTokenRepository
	Email        email.Sender

	VerifyLinkBaseURL string
	ResetLinkBaseURL  string

	VerifyTokenTTL time.Duration
	ResetTokenTTL  time.Duration

	Now func() time.Time

	// RevokeAllSessions is called by the service to invalidate every active
	// session for a user (password reset / change-password full revoke). The
	// caller wires this to sessionauth.Manager.DeleteAllForUser.
	RevokeAllSessions func(ctx context.Context, userID uuid.UUID) error

	// RevokeAllSessionsExcept is the variant that keeps a single session id
	// (used by ChangePassword from a logged-in browser). Wired to
	// sessionauth.Manager.DeleteAllForUserExcept.
	RevokeAllSessionsExcept func(ctx context.Context, userID uuid.UUID, keepID string) error
}

// IdentityService orchestrates auth flows.
type IdentityService struct {
	deps IdentityServiceDeps
}

// NewIdentityService builds the service. Defaults: VerifyTokenTTL=24h, ResetTokenTTL=1h, Now=time.Now.
func NewIdentityService(d IdentityServiceDeps) *IdentityService {
	if d.VerifyTokenTTL == 0 {
		d.VerifyTokenTTL = 24 * time.Hour
	}
	if d.ResetTokenTTL == 0 {
		d.ResetTokenTTL = time.Hour
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.RevokeAllSessions == nil {
		d.RevokeAllSessions = func(context.Context, uuid.UUID) error { return nil }
	}
	if d.RevokeAllSessionsExcept == nil {
		d.RevokeAllSessionsExcept = func(context.Context, uuid.UUID, string) error { return nil }
	}
	return &IdentityService{deps: d}
}

// RegisterInput is the request body for Register.
type RegisterInput struct {
	Email, Password, Name string
}

// Register creates a user, sets an initial password, issues a verify token,
// and sends the verify email. Returns the created user.
func (s *IdentityService) Register(ctx context.Context, in RegisterInput) (domain.User, error) {
	if err := validatePassword(in.Password); err != nil {
		return domain.User{}, err
	}
	if err := validateEmail(in.Email); err != nil {
		return domain.User{}, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return domain.User{}, fmt.Errorf("identity: %w: name required", errPolicyMisc)
	}

	user, err := s.deps.Users.Insert(ctx, in.Email, in.Name)
	if err != nil {
		// pass through ErrEmailAlreadyTaken
		return domain.User{}, err
	}

	hashedPwd, err := passwords.Hash(in.Password)
	if err != nil {
		return domain.User{}, fmt.Errorf("identity: hash password: %w", err)
	}
	if _, err := s.deps.AuthMethods.InsertPassword(ctx, user.ID, hashedPwd); err != nil {
		return domain.User{}, fmt.Errorf("identity: insert auth method: %w", err)
	}

	rawToken, hash, err := tokens.Generate()
	if err != nil {
		return domain.User{}, fmt.Errorf("identity: generate verify token: %w", err)
	}
	expires := s.deps.Now().Add(s.deps.VerifyTokenTTL).UTC()
	if err := s.deps.VerifyTokens.Insert(ctx, hash, user.ID, user.Email, expires); err != nil {
		return domain.User{}, fmt.Errorf("identity: store verify token: %w", err)
	}

	verifyURL := s.deps.VerifyLinkBaseURL + "?token=" + rawToken
	msg, err := email.RenderVerifyEmail(email.VerifyEmailData{
		ToAddress: user.Email,
		Name:      user.Name,
		VerifyURL: verifyURL,
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("identity: render verify email: %w", err)
	}
	if err := s.deps.Email.Send(ctx, msg); err != nil {
		return domain.User{}, fmt.Errorf("identity: send verify email: %w", err)
	}
	return user, nil
}

// errPolicyMisc is a fallback used to wrap policy violations not covered by sentinels.
var errPolicyMisc = errors.New("identity: policy violation")

func validatePassword(p string) error {
	if len(p) < 8 {
		return domain.ErrPasswordTooWeak
	}
	return nil
}

func validateEmail(e string) error {
	if !strings.Contains(e, "@") || !strings.Contains(e, ".") {
		return fmt.Errorf("identity: %w: invalid email", errPolicyMisc)
	}
	return nil
}

// uuid import marker (used by later methods); avoids "imported and not used" if Register is the only consumer.
var _ = uuid.Nil

// LoginInput is the login request payload.
type LoginInput struct {
	Email    string
	Password string
}

// Login validates credentials and returns the user.
// Returns ErrInvalidCredentials when email is unknown or password mismatches
// (with constant-time defense), and ErrEmailNotVerified only after a successful
// password match against an unverified user.
func (s *IdentityService) Login(ctx context.Context, in LoginInput) (domain.User, error) {
	user, err := s.deps.Users.FindByEmail(ctx, in.Email)
	if errors.Is(err, domain.ErrUserNotFound) {
		// Constant-time defense: pretend we have a hash to verify against, then fail.
		_, _ = passwords.Verify(in.Password, passwords.DummyHash)
		return domain.User{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("identity: lookup user: %w", err)
	}

	auth, err := s.deps.AuthMethods.FindForUser(ctx, user.ID, domain.AuthProviderPassword)
	if errors.Is(err, domain.ErrUserNotFound) || auth.PasswordHash == nil {
		_, _ = passwords.Verify(in.Password, passwords.DummyHash)
		return domain.User{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("identity: lookup auth method: %w", err)
	}

	ok, err := passwords.Verify(in.Password, *auth.PasswordHash)
	if err != nil {
		return domain.User{}, fmt.Errorf("identity: verify password: %w", err)
	}
	if !ok {
		return domain.User{}, domain.ErrInvalidCredentials
	}

	// Best-effort last-used touch; do not fail login on this.
	_ = s.deps.AuthMethods.TouchLastUsed(ctx, auth.ID)

	if !user.IsEmailVerified() {
		return domain.User{}, domain.ErrEmailNotVerified
	}
	if !user.IsActive() {
		return domain.User{}, domain.ErrInvalidCredentials
	}

	return user, nil
}

// VerifyEmail consumes a verify token and marks the user's email as verified.
//
// Returns ErrTokenNotFound, ErrTokenAlreadyUsed, or ErrTokenExpired for invalid
// tokens. Idempotent for already-consumed tokens belonging to verified users —
// they map to ErrTokenAlreadyUsed (transport returns 400 invalid_token, which is
// the correct privacy-preserving behaviour: don't tell callers whose mailbox
// it was).
func (s *IdentityService) VerifyEmail(ctx context.Context, rawToken string) error {
	hash, err := tokens.Hash(rawToken)
	if err != nil {
		return domain.ErrTokenNotFound
	}
	tok, err := s.deps.VerifyTokens.Find(ctx, hash)
	if errors.Is(err, domain.ErrTokenNotFound) {
		return domain.ErrTokenNotFound
	}
	if err != nil {
		return fmt.Errorf("identity: find verify token: %w", err)
	}
	if tok.IsConsumed() {
		return domain.ErrTokenAlreadyUsed
	}
	if tok.IsExpired(s.deps.Now()) {
		return domain.ErrTokenExpired
	}

	if err := s.deps.Users.MarkEmailVerified(ctx, tok.UserID); err != nil {
		return fmt.Errorf("identity: mark verified: %w", err)
	}
	if err := s.deps.VerifyTokens.Consume(ctx, hash); err != nil {
		return fmt.Errorf("identity: consume verify token: %w", err)
	}
	return nil
}

// ResendVerifyEmail issues a new verify token and sends a fresh email.
// Always returns nil even when the email does not match a user (anti-enumeration).
func (s *IdentityService) ResendVerifyEmail(ctx context.Context, addr string) error {
	user, err := s.deps.Users.FindByEmail(ctx, addr)
	if errors.Is(err, domain.ErrUserNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("identity: lookup user: %w", err)
	}
	if user.IsEmailVerified() {
		// Already verified; do not send again.
		return nil
	}
	rawToken, hash, err := tokens.Generate()
	if err != nil {
		return fmt.Errorf("identity: generate verify token: %w", err)
	}
	expires := s.deps.Now().Add(s.deps.VerifyTokenTTL).UTC()
	if err := s.deps.VerifyTokens.Insert(ctx, hash, user.ID, user.Email, expires); err != nil {
		return fmt.Errorf("identity: store verify token: %w", err)
	}
	verifyURL := s.deps.VerifyLinkBaseURL + "?token=" + rawToken
	msg, err := email.RenderVerifyEmail(email.VerifyEmailData{
		ToAddress: user.Email, Name: user.Name, VerifyURL: verifyURL,
	})
	if err != nil {
		return fmt.Errorf("identity: render verify email: %w", err)
	}
	if err := s.deps.Email.Send(ctx, msg); err != nil {
		return fmt.Errorf("identity: send verify email: %w", err)
	}
	return nil
}

// RequestPasswordReset issues a reset token + sends an email. Always returns nil
// regardless of whether the email matches a user (anti-enumeration).
func (s *IdentityService) RequestPasswordReset(ctx context.Context, addr string) error {
	user, err := s.deps.Users.FindByEmail(ctx, addr)
	if errors.Is(err, domain.ErrUserNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("identity: lookup user: %w", err)
	}
	if !user.IsEmailVerified() {
		// We do not allow resetting an unverified account; do not leak that fact.
		return nil
	}
	rawToken, hash, err := tokens.Generate()
	if err != nil {
		return fmt.Errorf("identity: generate reset token: %w", err)
	}
	expires := s.deps.Now().Add(s.deps.ResetTokenTTL).UTC()
	if err := s.deps.ResetTokens.Insert(ctx, hash, user.ID, expires); err != nil {
		return fmt.Errorf("identity: store reset token: %w", err)
	}
	resetURL := s.deps.ResetLinkBaseURL + "?token=" + rawToken
	msg, err := email.RenderPasswordResetEmail(email.PasswordResetEmailData{
		ToAddress: user.Email, Name: user.Name,
		ResetURL: resetURL, ExpiryMin: int(s.deps.ResetTokenTTL / time.Minute),
	})
	if err != nil {
		return fmt.Errorf("identity: render reset email: %w", err)
	}
	if err := s.deps.Email.Send(ctx, msg); err != nil {
		return fmt.Errorf("identity: send reset email: %w", err)
	}
	return nil
}

// ConfirmPasswordReset consumes a reset token, sets a new password hash,
// and revokes ALL sessions for the user.
func (s *IdentityService) ConfirmPasswordReset(ctx context.Context, rawToken, newPassword string) error {
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	hash, err := tokens.Hash(rawToken)
	if err != nil {
		return domain.ErrTokenNotFound
	}
	tok, err := s.deps.ResetTokens.Find(ctx, hash)
	if errors.Is(err, domain.ErrTokenNotFound) {
		return domain.ErrTokenNotFound
	}
	if err != nil {
		return fmt.Errorf("identity: find reset token: %w", err)
	}
	if tok.IsConsumed() {
		return domain.ErrTokenAlreadyUsed
	}
	if tok.IsExpired(s.deps.Now()) {
		return domain.ErrTokenExpired
	}
	encoded, err := passwords.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("identity: hash new password: %w", err)
	}
	if err := s.deps.AuthMethods.UpdatePassword(ctx, tok.UserID, encoded); err != nil {
		return fmt.Errorf("identity: update password: %w", err)
	}
	if err := s.deps.ResetTokens.Consume(ctx, hash); err != nil {
		return fmt.Errorf("identity: consume reset token: %w", err)
	}
	if err := s.deps.RevokeAllSessions(ctx, tok.UserID); err != nil {
		return fmt.Errorf("identity: revoke sessions: %w", err)
	}
	return nil
}

// ChangePasswordInput carries change-password parameters.
type ChangePasswordInput struct {
	UserID          uuid.UUID
	CurrentPassword string
	NewPassword     string
	KeepSessionID   string // current session id; revoked-except pivot
}

// ChangePassword verifies the current password, sets the new one, and revokes
// every session for this user EXCEPT KeepSessionID.
func (s *IdentityService) ChangePassword(ctx context.Context, in ChangePasswordInput) error {
	if err := validatePassword(in.NewPassword); err != nil {
		return err
	}
	auth, err := s.deps.AuthMethods.FindForUser(ctx, in.UserID, domain.AuthProviderPassword)
	if errors.Is(err, domain.ErrUserNotFound) || auth.PasswordHash == nil {
		return domain.ErrInvalidCredentials
	}
	if err != nil {
		return fmt.Errorf("identity: lookup auth method: %w", err)
	}

	ok, err := passwords.Verify(in.CurrentPassword, *auth.PasswordHash)
	if err != nil {
		return fmt.Errorf("identity: verify current password: %w", err)
	}
	if !ok {
		return domain.ErrInvalidCredentials
	}

	encoded, err := passwords.Hash(in.NewPassword)
	if err != nil {
		return fmt.Errorf("identity: hash new password: %w", err)
	}
	if err := s.deps.AuthMethods.UpdatePassword(ctx, in.UserID, encoded); err != nil {
		return fmt.Errorf("identity: update password: %w", err)
	}
	if err := s.deps.RevokeAllSessionsExcept(ctx, in.UserID, in.KeepSessionID); err != nil {
		return fmt.Errorf("identity: revoke other sessions: %w", err)
	}
	return nil
}

// GetMe returns the current user.
func (s *IdentityService) GetMe(ctx context.Context, userID uuid.UUID) (domain.User, error) {
	return s.deps.Users.FindByID(ctx, userID)
}

// UpdateProfileInput accepts editable fields.
type UpdateProfileInput struct {
	UserID uuid.UUID
	Name   string
}

// UpdateProfile updates the user's name.
func (s *IdentityService) UpdateProfile(ctx context.Context, in UpdateProfileInput) (domain.User, error) {
	if strings.TrimSpace(in.Name) == "" {
		return domain.User{}, fmt.Errorf("identity: %w: name required", errPolicyMisc)
	}
	return s.deps.Users.UpdateName(ctx, in.UserID, in.Name)
}
