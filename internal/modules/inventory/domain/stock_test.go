package domain_test

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
)

func TestReservationStatus_Values(t *testing.T) {
	assert.Equal(t, domain.ReservationStatus("held"), domain.StatusHeld)
	assert.Equal(t, domain.ReservationStatus("committed"), domain.StatusCommitted)
	assert.Equal(t, domain.ReservationStatus("released"), domain.StatusReleased)
}

func TestErrors_Prefixed(t *testing.T) {
	assert.Contains(t, domain.ErrInsufficientStock.Error(), "inventory:")
	assert.Contains(t, domain.ErrStockNotFound.Error(), "inventory:")
}
