-- name: CreateVariant :one
INSERT INTO catalog_variants (id, product_id, sku, size, color, price_cents)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListVariantsByProduct :many
SELECT * FROM catalog_variants WHERE product_id = $1 ORDER BY sku;

-- name: DeleteVariantsByProduct :exec
DELETE FROM catalog_variants WHERE product_id = $1;

-- name: ListImagesByProduct :many
SELECT * FROM catalog_images WHERE product_id = $1 ORDER BY position, created_at;

-- name: CreateImage :one
INSERT INTO catalog_images (id, product_id, variant_id, url, alt_text, position,
                            url_thumb, url_medium, url_large, storage_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: DeleteImagesByProduct :exec
DELETE FROM catalog_images WHERE product_id = $1;

-- name: DeleteImageByID :exec
DELETE FROM catalog_images WHERE id = $1;
