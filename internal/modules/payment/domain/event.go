package domain

// Event is a decoded webhook notification from the payment provider.
// Type is one of "paid" or "failed".
type Event struct {
	ID               string
	Type             string
	ProviderChargeID string
	AmountCents      int64
}
