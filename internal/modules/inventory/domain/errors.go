package domain

import "errors"

var (
	ErrInsufficientStock = errors.New("inventory: insufficient stock")
	ErrStockNotFound     = errors.New("inventory: stock not found")
	ErrStockConflict     = errors.New("inventory: stock version conflict")
)
