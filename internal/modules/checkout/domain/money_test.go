package domain_test

import (
	"testing"

	"github.com/danilloboing/marketplace-golang/internal/modules/checkout/domain"
	"github.com/stretchr/testify/assert"
)

func TestComputeDiscount_Fixed_CappedAtSubtotal(t *testing.T) {
	assert.Equal(t, int64(500), domain.ComputeDiscount(domain.Fixed, 500, 10000))
	assert.Equal(t, int64(10000), domain.ComputeDiscount(domain.Fixed, 99999, 10000)) // capped
}

func TestComputeDiscount_Percent_RoundHalfUp_Capped(t *testing.T) {
	// 10% of 19999 = 1999.9 → round-half-up → 2000
	assert.Equal(t, int64(2000), domain.ComputeDiscount(domain.Percent, 10, 19999))
	// 100% capped at subtotal
	assert.Equal(t, int64(5000), domain.ComputeDiscount(domain.Percent, 100, 5000))
}

func TestComputeTotal_NeverNegative(t *testing.T) {
	assert.Equal(t, int64(7990), domain.ComputeTotal(5000, 2990, 0))
	assert.Equal(t, int64(0), domain.ComputeTotal(5000, 0, 5000))
}
