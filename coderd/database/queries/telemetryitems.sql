-- name: InsertTelemetryItemIfNotExists :exec
INSERT INTO telemetry_items (key, value)
VALUES ($1, $2)
ON CONFLICT (key) DO NOTHING;

-- name: GetTelemetryItem :one
SELECT * FROM telemetry_items WHERE key = $1;

-- name: UpsertTelemetryItem :exec
INSERT INTO telemetry_items (key, value)
VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = NOW() WHERE telemetry_items.key = $1;

-- name: GetTelemetryItems :many
SELECT * FROM telemetry_items;
