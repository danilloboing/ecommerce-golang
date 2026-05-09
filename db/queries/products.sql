-- name: CreateProduct :one
INSERT INTO catalog_products (
    id, slug, name, description, brand, category_id,
    base_price_cents, currency, status
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, slug, name, description, brand, category_id, base_price_cents, currency, status, created_at, updated_at;

-- name: UpdateProduct :one
UPDATE catalog_products
SET slug = $2, name = $3, description = $4, brand = $5, category_id = $6,
    base_price_cents = $7, currency = $8, status = $9, updated_at = NOW()
WHERE id = $1
RETURNING id, slug, name, description, brand, category_id, base_price_cents, currency, status, created_at, updated_at;

-- name: DeleteProduct :exec
DELETE FROM catalog_products WHERE id = $1;

-- name: GetProductBySlug :one
SELECT id, slug, name, description, brand, category_id, base_price_cents, currency, status, created_at, updated_at
FROM catalog_products WHERE slug = $1;

-- name: GetProductByID :one
SELECT id, slug, name, description, brand, category_id, base_price_cents, currency, status, created_at, updated_at
FROM catalog_products WHERE id = $1;

-- name: ListPublishedProducts :many
SELECT id, slug, name, description, brand, category_id, base_price_cents, currency, status, created_at, updated_at
FROM catalog_products
WHERE status = 'published'
  AND (sqlc.narg('category_id')::uuid IS NULL OR category_id = sqlc.narg('category_id'))
  AND (sqlc.narg('min_price')::bigint IS NULL OR base_price_cents >= sqlc.narg('min_price'))
  AND (sqlc.narg('max_price')::bigint IS NULL OR base_price_cents <= sqlc.narg('max_price'))
  AND (sqlc.narg('brand')::text IS NULL OR brand = sqlc.narg('brand'))
ORDER BY created_at DESC, id DESC
LIMIT $1;

-- name: ListAdminProducts :many
SELECT id, slug, name, description, brand, category_id, base_price_cents, currency, status, created_at, updated_at
FROM catalog_products
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;
