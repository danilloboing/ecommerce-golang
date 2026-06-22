package application

import (
	"context"

	paymentdomain "github.com/danilloboing/marketplace-golang/internal/modules/payment/domain"
	paymentapp "github.com/danilloboing/marketplace-golang/internal/modules/payment/application"
)

// ReconcileRepository is the port the checkout reconciler depends on.
// It is implemented by *infrastructure.ReconcileRepo.
type ReconcileRepository interface {
	Apply(ctx context.Context, ev paymentdomain.Event) error
}

// Reconciler is a thin application-layer wrapper that satisfies
// payment/application.EventApplier. It delegates the one-tx reconciliation
// effect to the ReconcileRepository so the application layer remains
// infrastructure-agnostic.
type Reconciler struct {
	repo ReconcileRepository
}

// Verify that Reconciler satisfies the payment EventApplier interface at
// compile time. This import is one-directional: checkout → payment (never
// the reverse).
var _ paymentapp.EventApplier = (*Reconciler)(nil)

// NewReconciler builds a Reconciler from the given repository.
func NewReconciler(repo ReconcileRepository) *Reconciler {
	return &Reconciler{repo: repo}
}

// Apply delegates one payment event to the reconcile repository.
func (r *Reconciler) Apply(ctx context.Context, ev paymentdomain.Event) error {
	return r.repo.Apply(ctx, ev)
}
