package domain

// ComputeDiscount returns the discount in cents, round-half-up for percent,
// and capped at subtotal so it can never exceed the goods value (I4).
func ComputeDiscount(t CouponType, value, subtotalCents int64) int64 {
	var d int64
	switch t {
	case Fixed:
		d = value
	case Percent:
		d = (subtotalCents*value + 50) / 100 // round-half-up
	}
	if d > subtotalCents {
		d = subtotalCents
	}
	if d < 0 {
		d = 0
	}
	return d
}

// ComputeTotal = max(0, subtotal + shipping - discount) (I4).
func ComputeTotal(subtotal, shipping, discount int64) int64 {
	t := subtotal + shipping - discount
	if t < 0 {
		return 0
	}
	return t
}
