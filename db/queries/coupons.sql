-- name: CreateCoupon :one
INSERT INTO coupons (code, type, value, expires_at, usage_limit, min_order_cents)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetCouponByCode :one
SELECT * FROM coupons WHERE code = $1;

-- name: RedeemCoupon :one
UPDATE coupons SET used_count = used_count + 1
WHERE code = sqlc.arg(code) AND active
  AND (usage_limit IS NULL OR used_count < usage_limit)
  AND (expires_at IS NULL OR expires_at > now())
RETURNING *;

-- name: ReleaseCoupon :exec
UPDATE coupons SET used_count = used_count - 1
WHERE code = sqlc.arg(code) AND used_count > 0;
