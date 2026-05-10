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
