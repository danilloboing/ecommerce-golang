package domain

import (
	"time"

	"github.com/google/uuid"
)

// EmailVerifyToken is an opaque single-use token.
// Plaintext is sent via email; hash is what's stored.
type EmailVerifyToken struct {
	TokenHash  []byte
	UserID     uuid.UUID
	Email      string // snapshot at issuance
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

// IsConsumed returns true once the token has been used.
func (t EmailVerifyToken) IsConsumed() bool { return t.ConsumedAt != nil }

// IsExpired returns true if now >= ExpiresAt.
func (t EmailVerifyToken) IsExpired(now time.Time) bool {
	return !now.Before(t.ExpiresAt)
}

// PasswordResetToken mirrors EmailVerifyToken without the email snapshot.
type PasswordResetToken struct {
	TokenHash  []byte
	UserID     uuid.UUID
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

func (t PasswordResetToken) IsConsumed() bool { return t.ConsumedAt != nil }
func (t PasswordResetToken) IsExpired(now time.Time) bool {
	return !now.Before(t.ExpiresAt)
}
