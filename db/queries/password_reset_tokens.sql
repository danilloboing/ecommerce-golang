-- name: InsertPasswordResetToken :exec
INSERT INTO password_reset_tokens (token_hash, user_id, expires_at)
VALUES ($1, $2, $3);

-- name: FindPasswordResetToken :one
SELECT token_hash, user_id, expires_at, consumed_at, created_at
FROM password_reset_tokens
WHERE token_hash = $1;

-- name: ConsumePasswordResetToken :exec
UPDATE password_reset_tokens
SET consumed_at = now()
WHERE token_hash = $1 AND consumed_at IS NULL;

-- name: DeleteExpiredPasswordResetTokens :execrows
DELETE FROM password_reset_tokens
WHERE expires_at < now() - interval '7 days';
