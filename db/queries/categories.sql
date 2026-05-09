-- name: CreateCategory :one
INSERT INTO catalog_categories (id, slug, name, parent_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateCategory :one
UPDATE catalog_categories
SET name = $2, parent_id = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteCategory :exec
DELETE FROM catalog_categories WHERE id = $1;

-- name: GetCategoryBySlug :one
SELECT * FROM catalog_categories WHERE slug = $1;

-- name: GetCategoryByID :one
SELECT * FROM catalog_categories WHERE id = $1;

-- name: ListCategories :many
SELECT * FROM catalog_categories ORDER BY name;
