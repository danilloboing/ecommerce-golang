// Package domain holds the identity bounded context's value objects and errors.
package domain

import "errors"

// Sentinel errors. Map to HTTP at the transport boundary via error_mapping.go.
var (
	ErrInvalidCredentials = errors.New("identity: invalid credentials")
	ErrEmailNotVerified   = errors.New("identity: email not verified")
	ErrUserNotFound       = errors.New("identity: user not found")
	ErrEmailAlreadyTaken  = errors.New("identity: email already taken")
	ErrTokenExpired       = errors.New("identity: token expired")
	ErrTokenAlreadyUsed   = errors.New("identity: token already used")
	ErrTokenNotFound      = errors.New("identity: token not found")
	ErrSessionExpired     = errors.New("identity: session expired")
	ErrSessionNotFound    = errors.New("identity: session not found")
	ErrPasswordTooWeak    = errors.New("identity: password does not meet policy")
)
