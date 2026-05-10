-- name: InsertUser :one
INSERT INTO users (id, email, name, status, created_at, updated_at)
VALUES (gen_random_uuid(), $1, $2, 'active', now(), now())
RETURNING id, email, email_verified_at, name, status, created_at, updated_at;

-- name: FindUserByID :one
SELECT id, email, email_verified_at, name, status, created_at, updated_at
FROM users
WHERE id = $1;

-- name: FindUserByEmail :one
SELECT id, email, email_verified_at, name, status, created_at, updated_at
FROM users
WHERE email = $1;

-- name: MarkUserEmailVerified :exec
UPDATE users
SET email_verified_at = now(),
    updated_at = now()
WHERE id = $1 AND email_verified_at IS NULL;

-- name: UpdateUserName :one
UPDATE users
SET name = $2,
    updated_at = now()
WHERE id = $1
RETURNING id, email, email_verified_at, name, status, created_at, updated_at;
