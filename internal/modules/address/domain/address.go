// Package domain holds address value types and invariants.
package domain

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var postalPattern = regexp.MustCompile(`^[0-9]{8}$`)

// Address is a user's shipping address.
type Address struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	RecipientName string
	PostalCode    string
	Street        string
	Number        string
	Complement    *string
	Neighborhood  string
	City          string
	State         string
	IsDefault     bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Validate enforces required fields, 8-digit postal code, and 2-letter state.
func Validate(a Address) error {
	if blank(a.RecipientName) || blank(a.Street) || blank(a.Number) ||
		blank(a.Neighborhood) || blank(a.City) {
		return ErrInvalidAddress
	}
	if !postalPattern.MatchString(a.PostalCode) {
		return ErrInvalidAddress
	}
	if len(a.State) != 2 {
		return ErrInvalidAddress
	}
	return nil
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }
