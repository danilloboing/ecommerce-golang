-- name: CreateCharge :one
INSERT INTO charges (order_id, provider, provider_charge_id, method, status, amount_cents, raw_payload)
VALUES ($1, $2, $3, $4, 'pending', $5, $6)
RETURNING *;

-- name: GetChargeByProviderID :one
SELECT * FROM charges WHERE provider = $1 AND provider_charge_id = $2;

-- name: GetChargeByOrder :one
SELECT * FROM charges WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1;

-- name: SetChargeStatus :exec
UPDATE charges SET status = sqlc.arg(status), updated_at = now() WHERE id = sqlc.arg(id);

-- name: HasPaidCharge :one
SELECT EXISTS (SELECT 1 FROM charges WHERE order_id = $1 AND status = 'paid');

-- name: InsertWebhookEvent :execrows
INSERT INTO payment_webhook_events (event_id, provider, charge_id)
VALUES ($1, $2, $3) ON CONFLICT (event_id) DO NOTHING;
