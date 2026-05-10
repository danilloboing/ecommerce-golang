package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/application"
	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/email"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- mocks ---

type fakeUserRepo struct{ mock.Mock }

func (f *fakeUserRepo) Insert(ctx context.Context, e, n string) (domain.User, error) {
	args := f.Called(ctx, e, n)
	return args.Get(0).(domain.User), args.Error(1)
}
func (f *fakeUserRepo) FindByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	args := f.Called(ctx, id)
	return args.Get(0).(domain.User), args.Error(1)
}
func (f *fakeUserRepo) FindByEmail(ctx context.Context, e string) (domain.User, error) {
	args := f.Called(ctx, e)
	return args.Get(0).(domain.User), args.Error(1)
}
func (f *fakeUserRepo) MarkEmailVerified(ctx context.Context, id uuid.UUID) error {
	return f.Called(ctx, id).Error(0)
}
func (f *fakeUserRepo) UpdateName(ctx context.Context, id uuid.UUID, name string) (domain.User, error) {
	args := f.Called(ctx, id, name)
	return args.Get(0).(domain.User), args.Error(1)
}

type fakeAuthRepo struct{ mock.Mock }

func (f *fakeAuthRepo) InsertPassword(ctx context.Context, uid uuid.UUID, hash string) (domain.AuthMethod, error) {
	args := f.Called(ctx, uid, hash)
	return args.Get(0).(domain.AuthMethod), args.Error(1)
}
func (f *fakeAuthRepo) FindForUser(ctx context.Context, uid uuid.UUID, p domain.AuthProvider) (domain.AuthMethod, error) {
	args := f.Called(ctx, uid, p)
	return args.Get(0).(domain.AuthMethod), args.Error(1)
}
func (f *fakeAuthRepo) UpdatePassword(ctx context.Context, uid uuid.UUID, hash string) error {
	return f.Called(ctx, uid, hash).Error(0)
}
func (f *fakeAuthRepo) TouchLastUsed(ctx context.Context, id uuid.UUID) error {
	return f.Called(ctx, id).Error(0)
}

type fakeVerifyRepo struct{ mock.Mock }

func (f *fakeVerifyRepo) Insert(ctx context.Context, h []byte, uid uuid.UUID, e string, exp time.Time) error {
	return f.Called(ctx, h, uid, e, exp).Error(0)
}
func (f *fakeVerifyRepo) Find(ctx context.Context, h []byte) (domain.EmailVerifyToken, error) {
	args := f.Called(ctx, h)
	return args.Get(0).(domain.EmailVerifyToken), args.Error(1)
}
func (f *fakeVerifyRepo) Consume(ctx context.Context, h []byte) error {
	return f.Called(ctx, h).Error(0)
}

type fakeResetRepo struct{ mock.Mock }

func (f *fakeResetRepo) Insert(ctx context.Context, h []byte, uid uuid.UUID, exp time.Time) error {
	return f.Called(ctx, h, uid, exp).Error(0)
}
func (f *fakeResetRepo) Find(ctx context.Context, h []byte) (domain.PasswordResetToken, error) {
	args := f.Called(ctx, h)
	return args.Get(0).(domain.PasswordResetToken), args.Error(1)
}
func (f *fakeResetRepo) Consume(ctx context.Context, h []byte) error {
	return f.Called(ctx, h).Error(0)
}

type fakeSender struct {
	sent []email.Message
}

func (f *fakeSender) Send(_ context.Context, msg email.Message) error {
	f.sent = append(f.sent, msg)
	return nil
}

// --- tests ---

func TestRegister_HappyPath_CreatesUserPasswordTokenAndSendsEmail(t *testing.T) {
	users := &fakeUserRepo{}
	auths := &fakeAuthRepo{}
	verify := &fakeVerifyRepo{}
	reset := &fakeResetRepo{}
	sender := &fakeSender{}

	uid := uuid.New()
	users.On("Insert", mock.Anything, "ana@example.com", "Ana").
		Return(domain.User{ID: uid, Email: "ana@example.com", Name: "Ana", Status: domain.UserStatusActive}, nil)
	auths.On("InsertPassword", mock.Anything, uid, mock.AnythingOfType("string")).
		Return(domain.AuthMethod{}, nil)
	verify.On("Insert", mock.Anything, mock.AnythingOfType("[]uint8"), uid, "ana@example.com", mock.AnythingOfType("time.Time")).
		Return(nil)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users:             users,
		AuthMethods:       auths,
		VerifyTokens:      verify,
		ResetTokens:       reset,
		Email:             sender,
		VerifyLinkBaseURL: "https://app.example/verify",
		ResetLinkBaseURL:  "https://app.example/reset",
		VerifyTokenTTL:    24 * time.Hour,
		ResetTokenTTL:     time.Hour,
		Now:               func() time.Time { return time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC) },
	})

	u, err := svc.Register(context.Background(), application.RegisterInput{
		Email:    "ana@example.com",
		Password: "S3cretPass!",
		Name:     "Ana",
	})
	require.NoError(t, err)
	assert.Equal(t, uid, u.ID)
	require.Len(t, sender.sent, 1)
	assert.Contains(t, sender.sent[0].TextBody, "https://app.example/verify?token=")
	users.AssertExpectations(t)
	auths.AssertExpectations(t)
	verify.AssertExpectations(t)
}

func TestRegister_RejectsShortPassword(t *testing.T) {
	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: &fakeUserRepo{}, AuthMethods: &fakeAuthRepo{},
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: &fakeResetRepo{},
		Email:             &fakeSender{},
		VerifyLinkBaseURL: "https://app.example/verify", ResetLinkBaseURL: "https://app.example/reset",
		Now: time.Now,
	})

	_, err := svc.Register(context.Background(), application.RegisterInput{
		Email: "ana@example.com", Password: "short", Name: "Ana",
	})
	require.ErrorIs(t, err, domain.ErrPasswordTooWeak)
}

func TestRegister_PropagatesEmailDuplicate(t *testing.T) {
	users := &fakeUserRepo{}
	users.On("Insert", mock.Anything, mock.Anything, mock.Anything).
		Return(domain.User{}, domain.ErrEmailAlreadyTaken)

	svc := application.NewIdentityService(application.IdentityServiceDeps{
		Users: users, AuthMethods: &fakeAuthRepo{},
		VerifyTokens: &fakeVerifyRepo{}, ResetTokens: &fakeResetRepo{},
		Email:             &fakeSender{},
		VerifyLinkBaseURL: "https://app.example/verify", ResetLinkBaseURL: "https://app.example/reset",
		Now: time.Now,
	})

	_, err := svc.Register(context.Background(), application.RegisterInput{
		Email: "ana@example.com", Password: "S3cretPass!", Name: "Ana",
	})
	require.ErrorIs(t, err, domain.ErrEmailAlreadyTaken)
}
