// Package infrastructure adapts sqlc to the inventory domain.
package infrastructure

import (
	"github.com/danilloboing/marketplace-golang/internal/modules/inventory/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

func mapStock(r queries.InventoryStock) domain.Stock {
	return domain.Stock{VariantID: r.VariantID, Available: int(r.Available), Reserved: int(r.Reserved), Version: int(r.Version)}
}
