package domain

type OrderStatus string

const (
	PendingPayment    OrderStatus = "pending_payment"
	Paid              OrderStatus = "paid"
	PaymentFailed     OrderStatus = "payment_failed"
	Expired           OrderStatus = "expired"
	PaidAwaitingStock OrderStatus = "paid_awaiting_stock"
)

var allowed = map[OrderStatus]map[OrderStatus]bool{
	PendingPayment: {Paid: true, PaymentFailed: true, Expired: true},
	Expired:        {Paid: true, PaidAwaitingStock: true},
}

// CanTransition reports whether from→to is a permitted order transition (§6).
func CanTransition(from, to OrderStatus) bool { return allowed[from][to] }
