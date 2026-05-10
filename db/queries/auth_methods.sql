-- name: InsertAuthMethodPassword :one
INSERT INTO auth_methods (id, user_id, provider, password_hash, created_at)
VALUES (gen_random_uuid(), $1, 'password', $2, now())
RETURNING id, user_id, provider, password_hash, provider_subject, created_at, last_used_at;

-- name: FindAuthMethodByUserAndProvider :one
SELECT id, user_id, provider, password_hash, provider_subject, created_at, last_used_at
FROM auth_methods
WHERE user_id = $1 AND provider = $2;

-- name: UpdateAuthMethodPassword :exec
UPDATE auth_methods
SET password_hash = $2,
    last_used_at = now()
WHERE user_id = $1 AND provider = 'password';

-- name: TouchAuthMethodLastUsed :exec
UPDATE auth_methods
SET last_used_at = now()
WHERE id = $1;
