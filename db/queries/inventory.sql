-- name: UpsertStock :one
INSERT INTO inventory_stock (variant_id, available, reserved, version)
VALUES ($1, $2, 0, 0)
ON CONFLICT (variant_id) DO UPDATE
SET available = EXCLUDED.available, version = inventory_stock.version + 1, updated_at = now()
WHERE inventory_stock.version = sqlc.arg(expected_version)
RETURNING *;

-- name: GetStock :one
SELECT * FROM inventory_stock WHERE variant_id = $1;

-- name: ReserveStock :one
UPDATE inventory_stock
SET available = available - sqlc.arg(qty), reserved = reserved + sqlc.arg(qty),
    version = version + 1, updated_at = now()
WHERE variant_id = sqlc.arg(variant_id) AND available >= sqlc.arg(qty)
RETURNING *;

-- name: CommitReservedStock :exec
UPDATE inventory_stock
SET reserved = reserved - sqlc.arg(qty), version = version + 1, updated_at = now()
WHERE variant_id = sqlc.arg(variant_id) AND reserved >= sqlc.arg(qty);

-- name: ReleaseReservedStock :exec
UPDATE inventory_stock
SET available = available + sqlc.arg(qty), reserved = reserved - sqlc.arg(qty),
    version = version + 1, updated_at = now()
WHERE variant_id = sqlc.arg(variant_id) AND reserved >= sqlc.arg(qty);

-- name: CreateReservation :one
INSERT INTO stock_reservations (order_id, variant_id, quantity, status, expires_at)
VALUES ($1, $2, $3, 'held', $4)
RETURNING *;

-- name: ListReservationsByOrder :many
SELECT * FROM stock_reservations WHERE order_id = $1;

-- name: SetReservationStatus :execrows
UPDATE stock_reservations SET status = sqlc.arg(new_status)
WHERE order_id = sqlc.arg(order_id) AND status = 'held';

-- name: ListExpiredHeldOrderIDs :many
SELECT DISTINCT order_id FROM stock_reservations
WHERE status = 'held' AND expires_at < $1;
