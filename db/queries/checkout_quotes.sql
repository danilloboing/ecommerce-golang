-- name: CreateQuote :one
INSERT INTO checkout_quotes (user_id, cart_fingerprint, lines_snapshot, shipping_snapshot,
                             coupon_code, subtotal_cents, shipping_cents, discount_cents,
                             total_cents, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetUserQuote :one
SELECT * FROM checkout_quotes WHERE id = $1 AND user_id = $2;

-- name: GetIdempotencyKey :one
SELECT * FROM idempotency_keys WHERE user_id = $1 AND key = $2;

-- name: PutIdempotencyKey :exec
INSERT INTO idempotency_keys (user_id, key, request_hash, order_id, response)
VALUES ($1, $2, $3, $4, $5);
