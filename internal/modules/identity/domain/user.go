package domain

import (
	"time"

	"github.com/google/uuid"
)

// UserStatus values match the DB CHECK constraint.
type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusSuspended UserStatus = "suspended"
	UserStatusDeleted   UserStatus = "deleted"
)

// User is the identity aggregate root.
type User struct {
	ID              uuid.UUID
	Email           string
	EmailVerifiedAt *time.Time
	Name            string
	Status          UserStatus
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// IsEmailVerified returns true when EmailVerifiedAt is set.
func (u User) IsEmailVerified() bool { return u.EmailVerifiedAt != nil }

// IsActive returns true when Status == active.
func (u User) IsActive() bool { return u.Status == UserStatusActive }
