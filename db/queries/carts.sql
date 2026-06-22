-- name: GetActiveCartByUser :one
SELECT * FROM carts WHERE user_id = $1 AND status = 'active';

-- name: GetActiveCartByAnon :one
SELECT * FROM carts WHERE anon_session_id = $1 AND status = 'active';

-- name: CreateUserCart :one
INSERT INTO carts (id, user_id, status)
VALUES (gen_random_uuid(), $1, 'active')
RETURNING *;

-- name: CreateAnonCart :one
INSERT INTO carts (id, anon_session_id, status)
VALUES (gen_random_uuid(), $1, 'active')
RETURNING *;

-- name: SetCartStatus :exec
UPDATE carts SET status = $2, updated_at = now() WHERE id = $1;

-- name: DeleteAbandonedCarts :execrows
UPDATE carts SET status = 'abandoned', updated_at = now()
WHERE status = 'active' AND updated_at < $1 AND user_id IS NULL;
