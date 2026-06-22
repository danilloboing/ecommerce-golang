// Package infrastructure adapts sqlc queries to the address domain.
package infrastructure

import (
	"github.com/danilloboing/marketplace-golang/internal/modules/address/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

func mapAddress(row queries.Address) domain.Address {
	return domain.Address{
		ID:            row.ID,
		UserID:        row.UserID,
		RecipientName: row.RecipientName,
		PostalCode:    row.PostalCode,
		Street:        row.Street,
		Number:        row.Number,
		Complement:    row.Complement,
		Neighborhood:  row.Neighborhood,
		City:          row.City,
		State:         row.State,
		IsDefault:     row.IsDefault,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}
