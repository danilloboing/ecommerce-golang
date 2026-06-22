-- name: GetVariantUnitPrice :one
SELECT COALESCE(cv.price_cents, cp.base_price_cents)::bigint AS unit_price_cents
FROM catalog_variants cv
JOIN catalog_products cp ON cp.id = cv.product_id
WHERE cv.id = $1;

-- name: UpsertCartItem :one
INSERT INTO cart_items (id, cart_id, variant_id, quantity, unit_price_cents)
VALUES (gen_random_uuid(), $1, $2, $3, $4)
ON CONFLICT (cart_id, variant_id) DO UPDATE
SET quantity = LEAST(cart_items.quantity + EXCLUDED.quantity, 99),
    unit_price_cents = EXCLUDED.unit_price_cents,
    updated_at = now()
RETURNING *;

-- name: ListCartItems :many
SELECT * FROM cart_items WHERE cart_id = $1 ORDER BY added_at;

-- name: GetCartItemByID :one
SELECT ci.* FROM cart_items ci
JOIN carts c ON c.id = ci.cart_id
WHERE ci.id = $1 AND c.id = $2;

-- name: UpdateCartItemQuantity :one
UPDATE cart_items SET quantity = $3, updated_at = now()
WHERE id = $1 AND cart_id = $2
RETURNING *;

-- name: DeleteCartItem :execrows
DELETE FROM cart_items WHERE id = $1 AND cart_id = $2;

-- name: DeleteCartItemsByCart :exec
DELETE FROM cart_items WHERE cart_id = $1;

-- name: CountActiveItems :one
SELECT COUNT(*) FROM cart_items WHERE cart_id = $1;
