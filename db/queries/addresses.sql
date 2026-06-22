-- name: CreateAddress :one
INSERT INTO addresses (
    id, user_id, recipient_name, postal_code, street, number,
    complement, neighborhood, city, state, is_default
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetAddressByID :one
SELECT * FROM addresses WHERE id = $1 AND user_id = $2;

-- name: ListAddressesByUser :many
SELECT * FROM addresses WHERE user_id = $1 ORDER BY is_default DESC, created_at DESC;

-- name: UpdateAddress :one
UPDATE addresses
SET recipient_name = $3, postal_code = $4, street = $5, number = $6,
    complement = $7, neighborhood = $8, city = $9, state = $10, updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteAddress :execrows
DELETE FROM addresses WHERE id = $1 AND user_id = $2;

-- name: ClearDefaultAddress :exec
UPDATE addresses SET is_default = FALSE, updated_at = now()
WHERE user_id = $1 AND is_default = TRUE;

-- name: SetDefaultAddress :one
UPDATE addresses SET is_default = TRUE, updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING *;
