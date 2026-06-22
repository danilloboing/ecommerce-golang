-- name: CreateOrder :one
INSERT INTO orders (id, user_id, status, subtotal_cents, shipping_cents, discount_cents,
                    total_cents, coupon_code, address_snapshot, shipping_snapshot)
VALUES ($1, $2, 'pending_payment', $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetOrderByID :one
SELECT * FROM orders WHERE id = $1;

-- name: GetUserOrderByID :one
SELECT * FROM orders WHERE id = $1 AND user_id = $2;

-- name: ListOrdersByUser :many
SELECT * FROM orders WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2;

-- name: TransitionOrderStatus :execrows
UPDATE orders SET status = sqlc.arg(to_status), updated_at = now()
WHERE id = sqlc.arg(id) AND status = sqlc.arg(from_status);

-- name: SetOrderStatus :exec
UPDATE orders SET status = sqlc.arg(status), updated_at = now() WHERE id = sqlc.arg(id);

-- name: CreateOrderItem :one
INSERT INTO order_items (order_id, variant_id, quantity, unit_price_cents, product_snapshot)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListOrderItems :many
SELECT * FROM order_items WHERE order_id = $1 ORDER BY created_at;

-- name: RecordTransition :exec
INSERT INTO order_status_transitions (order_id, from_status, to_status, reason, actor)
VALUES ($1, $2, $3, $4, $5);
