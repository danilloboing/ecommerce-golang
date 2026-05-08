// Package infrastructure adapts sqlc-generated queries to the catalog domain.
package infrastructure

import (
	"time"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// productRow is a flat view of a catalog_products row used by mappers.
// All sqlc Row types share these fields (modulo extra per-query columns
// like SearchProductsRow.Rank), so mappers convert into this struct first.
type productRow struct {
	ID             uuid.UUID
	Slug           string
	Name           string
	Description    string
	Brand          string
	CategoryID     uuid.UUID
	BasePriceCents int64
	Currency       string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func fromGetBySlug(r queries.GetProductBySlugRow) productRow {
	return productRow{
		ID: r.ID, Slug: r.Slug, Name: r.Name, Description: r.Description, Brand: r.Brand,
		CategoryID: r.CategoryID, BasePriceCents: r.BasePriceCents, Currency: r.Currency,
		Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func fromGetByID(r queries.GetProductByIDRow) productRow {
	return productRow{
		ID: r.ID, Slug: r.Slug, Name: r.Name, Description: r.Description, Brand: r.Brand,
		CategoryID: r.CategoryID, BasePriceCents: r.BasePriceCents, Currency: r.Currency,
		Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func fromListPublished(r queries.ListPublishedProductsRow) productRow {
	return productRow{
		ID: r.ID, Slug: r.Slug, Name: r.Name, Description: r.Description, Brand: r.Brand,
		CategoryID: r.CategoryID, BasePriceCents: r.BasePriceCents, Currency: r.Currency,
		Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func fromListAdmin(r queries.ListAdminProductsRow) productRow {
	return productRow{
		ID: r.ID, Slug: r.Slug, Name: r.Name, Description: r.Description, Brand: r.Brand,
		CategoryID: r.CategoryID, BasePriceCents: r.BasePriceCents, Currency: r.Currency,
		Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func fromSearch(r queries.SearchProductsRow) productRow {
	return productRow{
		ID: r.ID, Slug: r.Slug, Name: r.Name, Description: r.Description, Brand: r.Brand,
		CategoryID: r.CategoryID, BasePriceCents: r.BasePriceCents, Currency: r.Currency,
		Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func mapProduct(row productRow, variants []queries.CatalogVariant, images []queries.CatalogImage) (domain.Product, error) {
	slug, err := domain.ParseSlug(row.Slug)
	if err != nil {
		return domain.Product{}, err
	}
	price, err := domain.NewMoney(row.BasePriceCents, row.Currency)
	if err != nil {
		return domain.Product{}, err
	}

	mappedVariants := make([]domain.Variant, 0, len(variants))
	for _, v := range variants {
		var priceOverride *domain.Money
		if v.PriceCents != nil {
			po, err := domain.NewMoney(*v.PriceCents, row.Currency)
			if err != nil {
				return domain.Product{}, err
			}
			priceOverride = &po
		}
		mappedVariants = append(mappedVariants, domain.Variant{
			ID:    v.ID,
			SKU:   v.Sku,
			Size:  v.Size,
			Color: v.Color,
			Price: priceOverride,
		})
	}

	mappedImages := make([]domain.Image, 0, len(images))
	for _, img := range images {
		mappedImages = append(mappedImages, domain.Image{
			ID:       img.ID,
			URL:      img.Url,
			Position: int(img.Position),
			AltText:  img.AltText,
		})
	}

	return domain.NewProduct(domain.NewProductInput{
		ID:          row.ID,
		Slug:        slug,
		Name:        row.Name,
		Description: row.Description,
		Brand:       row.Brand,
		CategoryID:  row.CategoryID,
		BasePrice:   price,
		Status:      domain.ProductStatus(row.Status),
		Variants:    mappedVariants,
		Images:      mappedImages,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	})
}

func mapCategory(row queries.CatalogCategory) (domain.Category, error) {
	slug, err := domain.ParseSlug(row.Slug)
	if err != nil {
		return domain.Category{}, err
	}
	var parent *uuid.UUID
	if row.ParentID != nil {
		p := *row.ParentID
		parent = &p
	}
	return domain.NewCategory(domain.NewCategoryInput{
		ID:       row.ID,
		Slug:     slug,
		Name:     row.Name,
		ParentID: parent,
	})
}
