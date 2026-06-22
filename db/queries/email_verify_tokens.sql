-- name: InsertEmailVerifyToken :exec
INSERT INTO email_verify_tokens (token_hash, user_id, email, expires_at)
VALUES ($1, $2, $3, $4);

-- name: FindEmailVerifyToken :one
SELECT token_hash, user_id, email, expires_at, consumed_at, created_at
FROM email_verify_tokens
WHERE token_hash = $1;

-- name: ConsumeEmailVerifyToken :exec
UPDATE email_verify_tokens
SET consumed_at = now()
WHERE token_hash = $1 AND consumed_at IS NULL;

-- name: DeleteExpiredEmailVerifyTokens :execrows
DELETE FROM email_verify_tokens
WHERE expires_at < now() - interval '7 days';
