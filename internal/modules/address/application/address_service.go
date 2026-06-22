package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
)

// AddressService orchestrates address use cases.
type AddressService struct {
	repo AddressRepository
}

// NewAddressService builds an AddressService.
func NewAddressService(repo AddressRepository) *AddressService {
	return &AddressService{repo: repo}
}

// CreateInput is the full create payload.
type CreateInput struct {
	UserID        uuid.UUID
	RecipientName string
	PostalCode    string
	Street        string
	Number        string
	Complement    *string
	Neighborhood  string
	City          string
	State         string
	IsDefault     bool
}

// UpdateInput is a partial update; nil fields are left unchanged.
type UpdateInput struct {
	UserID        uuid.UUID
	ID            uuid.UUID
	RecipientName *string
	PostalCode    *string
	Street        *string
	Number        *string
	Complement    *string
	Neighborhood  *string
	City          *string
	State         *string
}

// List returns the user's addresses (default first).
func (s *AddressService) List(ctx context.Context, userID uuid.UUID) ([]domain.Address, error) {
	return s.repo.List(ctx, userID)
}

// Create validates and persists a new address.
func (s *AddressService) Create(ctx context.Context, in CreateInput) (domain.Address, error) {
	a := domain.Address{
		ID:            uuid.New(),
		UserID:        in.UserID,
		RecipientName: in.RecipientName,
		PostalCode:    in.PostalCode,
		Street:        in.Street,
		Number:        in.Number,
		Complement:    in.Complement,
		Neighborhood:  in.Neighborhood,
		City:          in.City,
		State:         in.State,
		IsDefault:     in.IsDefault,
	}
	if err := domain.Validate(a); err != nil {
		return domain.Address{}, err
	}
	return s.repo.Create(ctx, a)
}

// Update fetches, applies provided fields, validates, and persists.
func (s *AddressService) Update(ctx context.Context, in UpdateInput) (domain.Address, error) {
	a, err := s.repo.GetByID(ctx, in.ID, in.UserID)
	if err != nil {
		return domain.Address{}, err
	}
	applyString(&a.RecipientName, in.RecipientName)
	applyString(&a.PostalCode, in.PostalCode)
	applyString(&a.Street, in.Street)
	applyString(&a.Number, in.Number)
	applyString(&a.Neighborhood, in.Neighborhood)
	applyString(&a.City, in.City)
	applyString(&a.State, in.State)
	if in.Complement != nil {
		a.Complement = in.Complement
	}
	if err := domain.Validate(a); err != nil {
		return domain.Address{}, err
	}
	return s.repo.Update(ctx, a)
}

// Delete removes an address scoped to the user.
func (s *AddressService) Delete(ctx context.Context, id, userID uuid.UUID) error {
	return s.repo.Delete(ctx, id, userID)
}

// SetDefault makes one address the user's default (atomic in the repo).
func (s *AddressService) SetDefault(ctx context.Context, id, userID uuid.UUID) (domain.Address, error) {
	return s.repo.SetDefault(ctx, id, userID)
}

func applyString(dst *string, src *string) {
	if src != nil {
		*dst = *src
	}
}
