// Package transport contains HTTP handlers for the identity module.
package transport

import (
	"time"

	"github.com/danilloboing/marketplace-golang/internal/modules/identity/domain"
	"github.com/google/uuid"
)

// UserResponse is the JSON shape of a user returned to the client.
type UserResponse struct {
	ID              uuid.UUID  `json:"id"`
	Email           string     `json:"email"`
	Name            string     `json:"name"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
}

func userResponse(u domain.User) UserResponse {
	return UserResponse{
		ID: u.ID, Email: u.Email, Name: u.Name,
		EmailVerifiedAt: u.EmailVerifiedAt,
	}
}
