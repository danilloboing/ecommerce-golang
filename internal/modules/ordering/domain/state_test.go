package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/danilloboing/marketplace-golang/internal/modules/ordering/domain"
)

func TestCanTransition(t *testing.T) {
	ok := [][2]domain.OrderStatus{
		{domain.PendingPayment, domain.Paid},
		{domain.PendingPayment, domain.PaymentFailed},
		{domain.PendingPayment, domain.Expired},
		{domain.Expired, domain.Paid},
		{domain.Expired, domain.PaidAwaitingStock},
	}
	for _, p := range ok {
		assert.Truef(t, domain.CanTransition(p[0], p[1]), "%s->%s should be allowed", p[0], p[1])
	}
	bad := [][2]domain.OrderStatus{
		{domain.Paid, domain.PendingPayment},
		{domain.Paid, domain.Expired},
		{domain.PaymentFailed, domain.Paid},
		{domain.PendingPayment, domain.PaidAwaitingStock},
	}
	for _, p := range bad {
		assert.Falsef(t, domain.CanTransition(p[0], p[1]), "%s->%s should be denied", p[0], p[1])
	}
}
