-- name: SearchProducts :many
SELECT id, slug, name, description, brand, category_id, base_price_cents, currency, status, created_at, updated_at,
    ts_rank(search_vector, plainto_tsquery('portuguese', $1)) AS rank
FROM catalog_products
WHERE status = 'published'
  AND (
    search_vector @@ plainto_tsquery('portuguese', $1)
    OR name % $1
  )
ORDER BY rank DESC, created_at DESC
LIMIT $2;
