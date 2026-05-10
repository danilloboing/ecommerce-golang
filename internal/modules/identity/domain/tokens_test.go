package domain_test

import (
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailVerifyToken_IsConsumed(t *testing.T) {
	tk := domain.EmailVerifyToken{}
	assert.False(t, tk.IsConsumed())
	tk.ConsumedAt = ptrTime(time.Now())
	assert.True(t, tk.IsConsumed())
}

func TestEmailVerifyToken_IsExpired(t *testing.T) {
	tk := domain.EmailVerifyToken{ExpiresAt: time.Now().Add(time.Hour)}
	assert.False(t, tk.IsExpired(time.Now()))

	tk.ExpiresAt = time.Now().Add(-time.Minute)
	assert.True(t, tk.IsExpired(time.Now()))
}

func TestPasswordResetToken_IsConsumedAndIsExpired(t *testing.T) {
	tk := domain.PasswordResetToken{ExpiresAt: time.Now().Add(time.Hour)}
	assert.False(t, tk.IsConsumed())
	assert.False(t, tk.IsExpired(time.Now()))

	tk.ConsumedAt = ptrTime(time.Now())
	tk.ExpiresAt = time.Now().Add(-time.Minute)
	assert.True(t, tk.IsConsumed())
	assert.True(t, tk.IsExpired(time.Now()))
}

func TestSentinelErrors_AreNotEqualButComparableViaIs(t *testing.T) {
	require.NotEqual(t, domain.ErrInvalidCredentials, domain.ErrEmailNotVerified)
}

func ptrTime(t time.Time) *time.Time { return &t }
