package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danilloboing/marketplace-golang/internal/modules/catalog/domain"
	"github.com/danilloboing/marketplace-golang/internal/platform/postgres/queries"
)

// Repository adapts sqlc-generated queries to the domain boundary.
type Repository struct {
	pool *pgxpool.Pool
	q    *queries.Queries
}

// New builds a Repository from a pgx pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: queries.New(pool)}
}

// ListPublished returns published products applying filters.
func (r *Repository) ListPublished(ctx context.Context, f domain.ListFilters) ([]domain.Product, error) {
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	params := queries.ListPublishedProductsParams{
		Limit: int32(limit),
	}
	if f.CategoryID != nil {
		params.CategoryID = f.CategoryID
	}
	if f.MinPriceCents != nil {
		params.MinPrice = f.MinPriceCents
	}
	if f.MaxPriceCents != nil {
		params.MaxPrice = f.MaxPriceCents
	}
	if f.Brand != nil {
		params.Brand = f.Brand
	}

	rows, err := r.q.ListPublishedProducts(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("catalog repo: list published: %w", err)
	}

	out := make([]domain.Product, 0, len(rows))
	for _, row := range rows {
		p, err := r.hydrateProduct(ctx, fromListPublished(row))
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// GetBySlug returns a product by slug.
func (r *Repository) GetBySlug(ctx context.Context, slug domain.Slug) (domain.Product, error) {
	row, err := r.q.GetProductBySlug(ctx, slug.String())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Product{}, domain.ErrNotFound
		}
		return domain.Product{}, fmt.Errorf("catalog repo: get by slug: %w", err)
	}
	return r.hydrateProduct(ctx, fromGetBySlug(row))
}

// GetByID returns a product by id.
func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	row, err := r.q.GetProductByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Product{}, domain.ErrNotFound
		}
		return domain.Product{}, fmt.Errorf("catalog repo: get by id: %w", err)
	}
	return r.hydrateProduct(ctx, fromGetByID(row))
}

// Search performs free-text + filter query.
func (r *Repository) Search(ctx context.Context, q domain.SearchQuery) ([]domain.Product, error) {
	limit := q.Filters.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.q.SearchProducts(ctx, queries.SearchProductsParams{
		PlaintoTsquery: q.Query,
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("catalog repo: search: %w", err)
	}
	out := make([]domain.Product, 0, len(rows))
	for _, row := range rows {
		p, err := r.hydrateProduct(ctx, fromSearch(row))
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (r *Repository) hydrateProduct(ctx context.Context, row productRow) (domain.Product, error) {
	variants, err := r.q.ListVariantsByProduct(ctx, row.ID)
	if err != nil {
		return domain.Product{}, fmt.Errorf("catalog repo: list variants: %w", err)
	}
	images, err := r.q.ListImagesByProduct(ctx, row.ID)
	if err != nil {
		return domain.Product{}, fmt.Errorf("catalog repo: list images: %w", err)
	}
	return mapProduct(row, variants, images)
}

// List returns all categories.
func (r *Repository) List(ctx context.Context) ([]domain.Category, error) {
	rows, err := r.q.ListCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog repo: list categories: %w", err)
	}
	out := make([]domain.Category, 0, len(rows))
	for _, row := range rows {
		c, err := mapCategory(row)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// GetCategoryBySlug returns a category by slug.
func (r *Repository) GetCategoryBySlug(ctx context.Context, slug domain.Slug) (domain.Category, error) {
	row, err := r.q.GetCategoryBySlug(ctx, slug.String())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Category{}, domain.ErrNotFound
		}
		return domain.Category{}, fmt.Errorf("catalog repo: get category by slug: %w", err)
	}
	return mapCategory(row)
}

// Create persists a new product (with its variants and images) atomically.
func (r *Repository) Create(ctx context.Context, p domain.Product) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("catalog repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := r.q.WithTx(tx)
	if _, err := q.CreateProduct(ctx, queries.CreateProductParams{
		ID:             p.ID(),
		Slug:           p.Slug().String(),
		Name:           p.Name(),
		Description:    p.Description(),
		Brand:          p.Brand(),
		CategoryID:     p.CategoryID(),
		BasePriceCents: p.BasePrice().AmountCents(),
		Currency:       p.BasePrice().Currency(),
		Status:         string(p.Status()),
	}); err != nil {
		return fmt.Errorf("catalog repo: create product: %w", err)
	}

	if err := persistChildren(ctx, q, p); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Update replaces product attributes and rebuilds variants/images atomically.
func (r *Repository) Update(ctx context.Context, p domain.Product) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("catalog repo: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := r.q.WithTx(tx)
	if _, err := q.UpdateProduct(ctx, queries.UpdateProductParams{
		ID:             p.ID(),
		Slug:           p.Slug().String(),
		Name:           p.Name(),
		Description:    p.Description(),
		Brand:          p.Brand(),
		CategoryID:     p.CategoryID(),
		BasePriceCents: p.BasePrice().AmountCents(),
		Currency:       p.BasePrice().Currency(),
		Status:         string(p.Status()),
	}); err != nil {
		return fmt.Errorf("catalog repo: update product: %w", err)
	}

	if err := q.DeleteVariantsByProduct(ctx, p.ID()); err != nil {
		return fmt.Errorf("catalog repo: delete variants: %w", err)
	}
	if err := q.DeleteImagesByProduct(ctx, p.ID()); err != nil {
		return fmt.Errorf("catalog repo: delete images: %w", err)
	}

	if err := persistChildren(ctx, q, p); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func persistChildren(ctx context.Context, q *queries.Queries, p domain.Product) error {
	for _, v := range p.Variants() {
		var priceCents *int64
		if v.Price != nil {
			pc := v.Price.AmountCents()
			priceCents = &pc
		}
		if _, err := q.CreateVariant(ctx, queries.CreateVariantParams{
			ID:         v.ID,
			ProductID:  p.ID(),
			Sku:        v.SKU,
			Size:       v.Size,
			Color:      v.Color,
			PriceCents: priceCents,
		}); err != nil {
			return fmt.Errorf("catalog repo: create variant: %w", err)
		}
	}

	for _, img := range p.Images() {
		params := queries.CreateImageParams{
			ID:        img.ID,
			ProductID: p.ID(),
			Url:       img.URL,
			AltText:   img.AltText,
			Position:  int32(img.Position),
		}
		if img.Variants != nil {
			urls := img.Variants.URLs()
			thumb, medium, large := urls.Thumb, urls.Medium, urls.Large
			params.UrlThumb = &thumb
			params.UrlMedium = &medium
			params.UrlLarge = &large
		}
		if img.StorageKey != "" {
			key := img.StorageKey
			params.StorageKey = &key
		}
		if _, err := q.CreateImage(ctx, params); err != nil {
			return fmt.Errorf("catalog repo: create image: %w", err)
		}
	}
	return nil
}

// Delete removes a product (cascade deletes variants/images via FK).
func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.q.DeleteProduct(ctx, id); err != nil {
		return fmt.Errorf("catalog repo: delete product: %w", err)
	}
	return nil
}

// CreateCategory persists a new category.
func (r *Repository) CreateCategory(ctx context.Context, c domain.Category) error {
	parent := c.ParentID()
	_, err := r.q.CreateCategory(ctx, queries.CreateCategoryParams{
		ID:       c.ID(),
		Slug:     c.Slug().String(),
		Name:     c.Name(),
		ParentID: parent,
	})
	if err != nil {
		return fmt.Errorf("catalog repo: create category: %w", err)
	}
	return nil
}

// UpdateCategory persists changes to a category.
func (r *Repository) UpdateCategory(ctx context.Context, c domain.Category) error {
	parent := c.ParentID()
	_, err := r.q.UpdateCategory(ctx, queries.UpdateCategoryParams{
		ID:       c.ID(),
		Name:     c.Name(),
		ParentID: parent,
	})
	if err != nil {
		return fmt.Errorf("catalog repo: update category: %w", err)
	}
	return nil
}

// DeleteCategory removes a category.
func (r *Repository) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	if err := r.q.DeleteCategory(ctx, id); err != nil {
		return fmt.Errorf("catalog repo: delete category: %w", err)
	}
	return nil
}

// AttachImage persists a single image row tied to an existing product.
func (r *Repository) AttachImage(ctx context.Context, productID uuid.UUID, img domain.Image) error {
	params := queries.CreateImageParams{
		ID:        img.ID,
		ProductID: productID,
		Url:       img.URL,
		AltText:   img.AltText,
		Position:  int32(img.Position),
	}
	if img.Variants != nil {
		urls := img.Variants.URLs()
		thumb, medium, large := urls.Thumb, urls.Medium, urls.Large
		params.UrlThumb = &thumb
		params.UrlMedium = &medium
		params.UrlLarge = &large
	}
	if img.StorageKey != "" {
		key := img.StorageKey
		params.StorageKey = &key
	}
	if _, err := r.q.CreateImage(ctx, params); err != nil {
		return fmt.Errorf("catalog repo: attach image: %w", err)
	}
	return nil
}
