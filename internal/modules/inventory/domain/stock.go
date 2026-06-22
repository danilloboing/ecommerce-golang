// Package domain holds inventory value types and invariants.
package domain

import (
	"time"
	"github.com/google/uuid"
)

type ReservationStatus string

const (
	StatusHeld      ReservationStatus = "held"
	StatusCommitted ReservationStatus = "committed"
	StatusReleased  ReservationStatus = "released"
)

// Stock is the sellable inventory for one variant.
type Stock struct {
	VariantID uuid.UUID
	Available int
	Reserved  int
	Version   int
}

// Reservation is a hold on stock tied to an order.
type Reservation struct {
	ID        uuid.UUID
	OrderID   uuid.UUID
	VariantID uuid.UUID
	Quantity  int
	Status    ReservationStatus
	ExpiresAt time.Time
}
