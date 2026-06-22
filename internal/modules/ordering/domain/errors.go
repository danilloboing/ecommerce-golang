package domain

import "errors"

// Sentinel errors for the ordering bounded context.
var (
	ErrOrderNotFound     = errors.New("ordering: order not found")
	ErrInvalidTransition = errors.New("ordering: invalid transition")
)
