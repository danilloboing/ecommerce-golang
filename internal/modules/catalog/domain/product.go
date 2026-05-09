package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// ProductStatus enumerates publication states.
type ProductStatus string

const (
	// ProductStatusDraft means the product is not yet visible publicly.
	ProductStatusDraft ProductStatus = "draft"
	// ProductStatusPublished means the product is browsable by customers.
	ProductStatusPublished ProductStatus = "published"
	// ProductStatusArchived means the product is hidden but kept for history.
	ProductStatusArchived ProductStatus = "archived"
)

// Variant represents a SKU (size+color combination).
type Variant struct {
	ID    uuid.UUID
	SKU   string
	Size  string
	Color string
	Price *Money // nil = inherit Product.BasePrice
}

// Image is a stored URL pointing to a product photo.
type Image struct {
	ID       uuid.UUID
	URL      string
	Position int
	AltText  string
}

// Product is the aggregate root of the catalog domain.
type Product struct {
	id          uuid.UUID
	slug        Slug
	name        string
	description string
	brand       string
	categoryID  uuid.UUID
	basePrice   Money
	status      ProductStatus
	variants    []Variant
	images      []Image
	createdAt   time.Time
	updatedAt   time.Time
}

// NewProductInput is the constructor input for Product.
type NewProductInput struct {
	ID          uuid.UUID
	Slug        Slug
	Name        string
	Description string
	Brand       string
	CategoryID  uuid.UUID
	BasePrice   Money
	Status      ProductStatus
	Variants    []Variant
	Images      []Image
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewProduct constructs a Product, validating invariants.
func NewProduct(in NewProductInput) (Product, error) {
	if strings.TrimSpace(in.Name) == "" {
		return Product{}, ErrInvalidProduct
	}
	if in.ID == uuid.Nil {
		return Product{}, ErrInvalidProduct
	}
	if in.CategoryID == uuid.Nil {
		return Product{}, ErrInvalidProduct
	}
	if in.Slug.IsZero() {
		return Product{}, ErrInvalidProduct
	}
	if !validProductStatus(in.Status) {
		return Product{}, ErrInvalidProduct
	}

	p := Product{
		id:          in.ID,
		slug:        in.Slug,
		name:        strings.TrimSpace(in.Name),
		description: strings.TrimSpace(in.Description),
		brand:       strings.TrimSpace(in.Brand),
		categoryID:  in.CategoryID,
		basePrice:   in.BasePrice,
		status:      in.Status,
		images:      append([]Image{}, in.Images...),
		createdAt:   in.CreatedAt,
		updatedAt:   in.UpdatedAt,
	}
	for _, v := range in.Variants {
		if err := p.AddVariant(v); err != nil {
			return Product{}, err
		}
	}
	return p, nil
}

func validProductStatus(s ProductStatus) bool {
	switch s {
	case ProductStatusDraft, ProductStatusPublished, ProductStatusArchived:
		return true
	default:
		return false
	}
}

// ID returns the product identifier.
func (p Product) ID() uuid.UUID { return p.id }

// Slug returns the product slug.
func (p Product) Slug() Slug { return p.slug }

// Name returns the product display name.
func (p Product) Name() string { return p.name }

// Description returns the product description.
func (p Product) Description() string { return p.description }

// Brand returns the brand name.
func (p Product) Brand() string { return p.brand }

// CategoryID returns the owning category.
func (p Product) CategoryID() uuid.UUID { return p.categoryID }

// BasePrice returns the default product price.
func (p Product) BasePrice() Money { return p.basePrice }

// Status returns the publication status.
func (p Product) Status() ProductStatus { return p.status }

// Variants returns a copy of the variant slice (avoids aliasing).
func (p Product) Variants() []Variant { return append([]Variant{}, p.variants...) }

// Images returns a copy of the image slice.
func (p Product) Images() []Image { return append([]Image{}, p.images...) }

// CreatedAt returns the creation timestamp.
func (p Product) CreatedAt() time.Time { return p.createdAt }

// UpdatedAt returns the last update timestamp.
func (p Product) UpdatedAt() time.Time { return p.updatedAt }

// AddVariant appends a variant, rejecting duplicates by SKU.
func (p *Product) AddVariant(v Variant) error {
	if strings.TrimSpace(v.SKU) == "" || v.ID == uuid.Nil {
		return ErrInvalidProduct
	}
	for _, existing := range p.variants {
		if existing.SKU == v.SKU {
			return ErrDuplicateSKU
		}
	}
	p.variants = append(p.variants, v)
	return nil
}

// AddImage appends an image preserving caller order.
func (p *Product) AddImage(img Image) error {
	if img.URL == "" || img.ID == uuid.Nil {
		return ErrInvalidProduct
	}
	p.images = append(p.images, img)
	return nil
}

// Category is a navigational hierarchy node in the catalog.
type Category struct {
	id       uuid.UUID
	slug     Slug
	name     string
	parentID *uuid.UUID
}

// NewCategoryInput is the constructor input.
type NewCategoryInput struct {
	ID       uuid.UUID
	Slug     Slug
	Name     string
	ParentID *uuid.UUID
}

// NewCategory constructs a Category.
func NewCategory(in NewCategoryInput) (Category, error) {
	if in.ID == uuid.Nil {
		return Category{}, ErrInvalidCategory
	}
	if in.Slug.IsZero() {
		return Category{}, ErrInvalidCategory
	}
	if strings.TrimSpace(in.Name) == "" {
		return Category{}, ErrInvalidCategory
	}
	return Category{
		id:       in.ID,
		slug:     in.Slug,
		name:     strings.TrimSpace(in.Name),
		parentID: in.ParentID,
	}, nil
}

// ID returns the identifier.
func (c Category) ID() uuid.UUID { return c.id }

// Slug returns the category slug.
func (c Category) Slug() Slug { return c.slug }

// Name returns the human-readable name.
func (c Category) Name() string { return c.name }

// ParentID returns the parent identifier (nil for root).
func (c Category) ParentID() *uuid.UUID { return c.parentID }
