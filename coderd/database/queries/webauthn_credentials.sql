-- name: GetWebAuthnCredentialsByUserID :many
SELECT * FROM webauthn_credentials
WHERE user_id = $1
ORDER BY created_at ASC;

-- name: GetWebAuthnCredentialByID :one
SELECT * FROM webauthn_credentials
WHERE id = $1;

-- name: InsertWebAuthnCredential :one
INSERT INTO webauthn_credentials (
    id, user_id, credential_id, public_key, attestation_type,
    aaguid, sign_count, name
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: UpdateWebAuthnCredentialSignCount :exec
UPDATE webauthn_credentials
SET sign_count = $2, last_used_at = $3
WHERE id = $1;

-- name: DeleteWebAuthnCredential :exec
DELETE FROM webauthn_credentials
WHERE id = $1;
