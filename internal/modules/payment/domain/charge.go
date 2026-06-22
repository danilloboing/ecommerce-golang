// Package domain holds payment value types and invariants.
package domain

import "github.com/google/uuid"

// ChargeStatus is the lifecycle state of a payment charge.
type ChargeStatus string

// Charge lifecycle states.
const (
	ChargePending  ChargeStatus = "pending"
	ChargePaid     ChargeStatus = "paid"
	ChargeFailed   ChargeStatus = "failed"
	ChargeRefunded ChargeStatus = "refunded"
)

// Charge represents a payment attempt for an order.
type Charge struct {
	ID               uuid.UUID
	OrderID          uuid.UUID
	Provider         string
	ProviderChargeID string
	Method           string
	Status           ChargeStatus
	AmountCents      int64
}
