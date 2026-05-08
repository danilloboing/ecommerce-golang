package domain

import (
	"github.com/google/uuid"
)

// ListFilters narrows a product listing.
type ListFilters struct {
	CategoryID    *uuid.UUID
	Sizes         []string
	Colors        []string
	MinPriceCents *int64
	MaxPriceCents *int64
	Brand         *string
	Status        *ProductStatus
	Cursor        string // opaque, base64(uuid:created_at)
	Limit         int    // 1..100, default 20
}

// SearchQuery describes a free-text query plus filters.
type SearchQuery struct {
	Query   string
	Filters ListFilters
}
