package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
)

// AdminWriter combines product and category writers used by AdminService.
type AdminWriter interface {
	ProductWriter
	CategoryWriter
	GetByID(ctx context.Context, id uuid.UUID) (domain.Product, error)
}

// AdminService orchestrates catalog mutations.
type AdminService struct {
	repo AdminWriter
}

// NewAdminService builds an AdminService.
func NewAdminService(repo AdminWriter) *AdminService {
	return &AdminService{repo: repo}
}

// VariantInput is the admin-facing variant payload.
type VariantInput struct {
	SKU        string
	Size       string
	Color      string
	PriceCents *int64
}

// ImageInput is the admin-facing image payload (URL only in this slice).
type ImageInput struct {
	URL      string
	AltText  string
	Position int
}

// CreateProductInput captures everything required to create a product.
type CreateProductInput struct {
	Slug           string
	Name           string
	Description    string
	Brand          string
	CategoryID     uuid.UUID
	BasePriceCents int64
	Currency       string
	Status         string
	Variants       []VariantInput
	Images         []ImageInput
}

// UpdateProductInput captures the editable fields of a product.
type UpdateProductInput = CreateProductInput

// CreateCategoryInput captures everything required to create a category.
type CreateCategoryInput struct {
	Slug     string
	Name     string
	ParentID *uuid.UUID
}

// CreateProduct validates and persists a new product.
func (s *AdminService) CreateProduct(ctx context.Context, in CreateProductInput) (domain.Product, error) {
	p, err := buildProduct(uuid.New(), in)
	if err != nil {
		return domain.Product{}, err
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return domain.Product{}, err
	}
	return p, nil
}

// UpdateProduct validates and persists changes to an existing product.
func (s *AdminService) UpdateProduct(ctx context.Context, id uuid.UUID, in UpdateProductInput) (domain.Product, error) {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return domain.Product{}, err
	}
	p, err := buildProduct(id, in)
	if err != nil {
		return domain.Product{}, err
	}
	if err := s.repo.Update(ctx, p); err != nil {
		return domain.Product{}, err
	}
	return p, nil
}

// DeleteProduct removes a product.
func (s *AdminService) DeleteProduct(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

// CreateCategory validates and persists a new category.
func (s *AdminService) CreateCategory(ctx context.Context, in CreateCategoryInput) (domain.Category, error) {
	slug, err := domain.ParseSlug(in.Slug)
	if err != nil {
		slug, err = domain.SlugFromTitle(in.Slug)
		if err != nil {
			return domain.Category{}, err
		}
	}
	c, err := domain.NewCategory(domain.NewCategoryInput{
		ID:       uuid.New(),
		Slug:     slug,
		Name:     in.Name,
		ParentID: in.ParentID,
	})
	if err != nil {
		return domain.Category{}, err
	}
	if err := s.repo.CreateCategory(ctx, c); err != nil {
		return domain.Category{}, err
	}
	return c, nil
}

func buildProduct(id uuid.UUID, in CreateProductInput) (domain.Product, error) {
	slug, err := domain.ParseSlug(in.Slug)
	if err != nil {
		slug, err = domain.SlugFromTitle(in.Slug)
		if err != nil {
			return domain.Product{}, err
		}
	}
	price, err := domain.NewMoney(in.BasePriceCents, in.Currency)
	if err != nil {
		return domain.Product{}, err
	}

	variants := make([]domain.Variant, 0, len(in.Variants))
	for _, v := range in.Variants {
		var priceMoney *domain.Money
		if v.PriceCents != nil {
			pm, err := domain.NewMoney(*v.PriceCents, in.Currency)
			if err != nil {
				return domain.Product{}, err
			}
			priceMoney = &pm
		}
		variants = append(variants, domain.Variant{
			ID:    uuid.New(),
			SKU:   v.SKU,
			Size:  v.Size,
			Color: v.Color,
			Price: priceMoney,
		})
	}

	images := make([]domain.Image, 0, len(in.Images))
	for _, img := range in.Images {
		images = append(images, domain.Image{
			ID:       uuid.New(),
			URL:      img.URL,
			AltText:  img.AltText,
			Position: img.Position,
		})
	}

	now := time.Now().UTC()
	return domain.NewProduct(domain.NewProductInput{
		ID:          id,
		Slug:        slug,
		Name:        in.Name,
		Description: in.Description,
		Brand:       in.Brand,
		CategoryID:  in.CategoryID,
		BasePrice:   price,
		Status:      domain.ProductStatus(in.Status),
		Variants:    variants,
		Images:      images,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}
