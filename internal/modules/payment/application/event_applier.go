package application

import (
	"context"

	"github.com/danilloboing/marketplace-golang/internal/modules/payment/domain"
)

// EventApplier is the port that processes decoded webhook events.
// It is implemented by the checkout reconciler (Task 20); payment never imports checkout.
type EventApplier interface {
	Apply(ctx context.Context, ev domain.Event) error
}
