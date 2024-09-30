-- name: GetCryptoKeys :many
SELECT *
FROM crypto_keys
WHERE secret IS NOT NULL;

-- name: GetCryptoKeysByFeature :many
SELECT *
FROM crypto_keys
WHERE feature = $1
AND secret IS NOT NULL
ORDER BY sequence DESC;

-- name: GetLatestCryptoKeyByFeature :one
SELECT *
FROM crypto_keys
WHERE feature = $1
ORDER BY sequence DESC
LIMIT 1;

-- name: GetCryptoKeyByFeatureAndSequence :one
SELECT *
FROM crypto_keys
WHERE feature = $1
  AND sequence = $2
  AND secret IS NOT NULL;

-- name: DeleteCryptoKey :one
UPDATE crypto_keys
SET secret = NULL, secret_key_id = NULL
WHERE feature = $1 AND sequence = $2 RETURNING *;

-- name: InsertCryptoKey :one
INSERT INTO crypto_keys (
    feature,
    sequence,
    secret,
    starts_at,
    secret_key_id
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5
) RETURNING *;

-- name: UpdateCryptoKeyDeletesAt :one
UPDATE crypto_keys
SET deletes_at = $3
WHERE feature = $1 AND sequence = $2 RETURNING *;
