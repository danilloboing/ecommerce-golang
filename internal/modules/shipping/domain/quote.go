// Package domain contains the core types for the shipping bounded context.
package domain

// QuoteRequest carries the inputs needed to request shipping quotes.
type QuoteRequest struct {
	PostalCode    string
	SubtotalCents int64
}

// Quote is a single shipping option returned by a provider.
type Quote struct {
	ServiceID  string
	Name       string
	PriceCents int64
	ETADays    int
}
