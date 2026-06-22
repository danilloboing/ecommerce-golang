package domain

import "errors"

// ErrQuoteUnavailable is returned when the provider cannot produce any quote
// for the given request (e.g. unsupported postal code or service outage).
var ErrQuoteUnavailable = errors.New("shipping: quote unavailable")
