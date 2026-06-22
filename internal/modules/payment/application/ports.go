// Package application contains payment use cases and ports.
package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/payment/domain"
)

// ChargeRequest carries the data required to initiate a new payment charge.
type ChargeRequest struct {
	OrderID     uuid.UUID
	AmountCents int64
	Method      string
}

// PaymentProvider is the port that payment gateway adapters must satisfy.
type PaymentProvider interface {
	CreateCharge(ctx context.Context, req ChargeRequest) (domain.Charge, error)
	VerifyWebhook(payload []byte, signature string) (domain.Event, error)
}

// ChargeRepository is the persistence contract for charges.
type ChargeRepository interface {
	Create(ctx context.Context, c domain.Charge) (domain.Charge, error)
}

// ChargeService orchestrates charge creation through the provider and repository.
type ChargeService struct {
	provider PaymentProvider
	repo     ChargeRepository
}

// NewChargeService builds a ChargeService.
func NewChargeService(p PaymentProvider, r ChargeRepository) *ChargeService {
	return &ChargeService{provider: p, repo: r}
}

// CreateCharge delegates charge creation to the provider, then persists via the repository.
func (s *ChargeService) CreateCharge(ctx context.Context, req ChargeRequest) (domain.Charge, error) {
	ch, err := s.provider.CreateCharge(ctx, req)
	if err != nil {
		return domain.Charge{}, err
	}
	return s.repo.Create(ctx, ch)
}
