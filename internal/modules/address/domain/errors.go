package domain

import "errors"

// Sentinel errors for the address bounded context.
var (
	ErrAddressNotFound = errors.New("address: not found")
	ErrInvalidAddress  = errors.New("address: invalid address")
	ErrInvalidCEP      = errors.New("address: invalid cep")
	ErrCEPNotFound     = errors.New("address: cep not found")
)
