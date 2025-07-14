-- name: GetOAuth2ProviderApps :many
SELECT
    oauth2_provider_apps.*,
    users.username,
    users.email
FROM oauth2_provider_apps
LEFT JOIN users ON oauth2_provider_apps.user_id = users.id
ORDER BY (oauth2_provider_apps.name, oauth2_provider_apps.id) ASC;

-- name: GetOAuth2ProviderAppByID :one
SELECT
    oauth2_provider_apps.*,
    users.username,
    users.email
FROM oauth2_provider_apps
LEFT JOIN users ON oauth2_provider_apps.user_id = users.id
WHERE oauth2_provider_apps.id = $1;

-- name: GetOAuth2ProviderAppsByOwnerID :many
SELECT
    oauth2_provider_apps.*,
    users.username,
    users.email
FROM oauth2_provider_apps
LEFT JOIN users ON oauth2_provider_apps.user_id = users.id
WHERE oauth2_provider_apps.user_id = $1
ORDER BY (oauth2_provider_apps.name, oauth2_provider_apps.id) ASC;

-- name: InsertOAuth2ProviderApp :one
INSERT INTO oauth2_provider_apps (
    id,
    created_at,
    updated_at,
    name,
    icon,
    redirect_uris,
    client_type,
    dynamically_registered,
    client_id_issued_at,
    client_secret_expires_at,
    grant_types,
    response_types,
    token_endpoint_auth_method,
    scope,
    contacts,
    client_uri,
    logo_uri,
    tos_uri,
    policy_uri,
    jwks_uri,
    jwks,
    software_id,
    software_version,
    registration_access_token,
    registration_client_uri,
    user_id
) VALUES(
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    $10,
    $11,
    $12,
    $13,
    $14,
    $15,
    $16,
    $17,
    $18,
    $19,
    $20,
    $21,
    $22,
    $23,
    $24,
    $25,
    $26
) RETURNING *;

-- name: UpdateOAuth2ProviderAppByID :one
UPDATE oauth2_provider_apps SET
    updated_at = $2,
    name = $3,
    icon = $4,
    redirect_uris = $5,
    client_type = $6,
    dynamically_registered = $7,
    client_secret_expires_at = $8,
    grant_types = $9,
    response_types = $10,
    token_endpoint_auth_method = $11,
    scope = $12,
    contacts = $13,
    client_uri = $14,
    logo_uri = $15,
    tos_uri = $16,
    policy_uri = $17,
    jwks_uri = $18,
    jwks = $19,
    software_id = $20,
    software_version = $21
WHERE id = $1 RETURNING *;

-- name: DeleteOAuth2ProviderAppByID :exec
DELETE FROM oauth2_provider_apps WHERE id = $1;

-- name: GetOAuth2ProviderAppSecretByID :one
SELECT * FROM oauth2_provider_app_secrets WHERE id = $1;

-- name: GetOAuth2ProviderAppSecretsByAppID :many
SELECT * FROM oauth2_provider_app_secrets WHERE app_id = $1 ORDER BY (created_at, id) ASC;

-- name: GetOAuth2ProviderAppSecretByPrefix :one
SELECT * FROM oauth2_provider_app_secrets WHERE secret_prefix = $1;

-- name: InsertOAuth2ProviderAppSecret :one
INSERT INTO oauth2_provider_app_secrets (
    id,
    created_at,
    secret_prefix,
    hashed_secret,
    display_secret,
    app_id,
    app_owner_user_id
) VALUES(
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7
) RETURNING *;

-- name: UpdateOAuth2ProviderAppSecretByID :one
UPDATE oauth2_provider_app_secrets SET
    last_used_at = $2
WHERE id = $1 RETURNING *;

-- name: DeleteOAuth2ProviderAppSecretByID :exec
DELETE FROM oauth2_provider_app_secrets WHERE id = $1;

-- name: GetOAuth2ProviderAppCodeByID :one
SELECT * FROM oauth2_provider_app_codes WHERE id = $1;

-- name: GetOAuth2ProviderAppCodeByPrefix :one
SELECT * FROM oauth2_provider_app_codes WHERE secret_prefix = $1;

-- name: ConsumeOAuth2ProviderAppCodeByPrefix :one
DELETE FROM oauth2_provider_app_codes
WHERE id = (
    SELECT c.id FROM oauth2_provider_app_codes c
    WHERE c.secret_prefix = $1 AND c.expires_at > NOW()
    LIMIT 1
)
RETURNING *;

-- name: InsertOAuth2ProviderAppCode :one
INSERT INTO oauth2_provider_app_codes (
    id,
    created_at,
    expires_at,
    secret_prefix,
    hashed_secret,
    app_id,
    user_id,
    resource_uri,
    code_challenge,
    code_challenge_method
) VALUES(
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    $10
) RETURNING *;

-- name: DeleteOAuth2ProviderAppCodeByID :exec
DELETE FROM oauth2_provider_app_codes WHERE id = $1;

-- name: DeleteOAuth2ProviderAppCodesByAppAndUserID :exec
DELETE FROM oauth2_provider_app_codes WHERE app_id = $1 AND user_id = $2;

-- name: InsertOAuth2ProviderAppToken :one
INSERT INTO oauth2_provider_app_tokens (
    id,
    created_at,
    expires_at,
    hash_prefix,
    refresh_hash,
    app_secret_id,
    api_key_id,
    user_id,
    audience
) VALUES(
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9
) RETURNING *;

-- name: GetOAuth2ProviderAppTokenByPrefix :one
SELECT * FROM oauth2_provider_app_tokens WHERE hash_prefix = $1;

-- name: GetOAuth2ProviderAppTokenByAPIKeyID :one
SELECT * FROM oauth2_provider_app_tokens WHERE api_key_id = $1;

-- name: GetOAuth2ProviderAppsByUserID :many
SELECT
  COUNT(DISTINCT oauth2_provider_app_tokens.id) as token_count,
  sqlc.embed(oauth2_provider_apps)
FROM oauth2_provider_app_tokens
  INNER JOIN oauth2_provider_app_secrets
    ON oauth2_provider_app_secrets.id = oauth2_provider_app_tokens.app_secret_id
  INNER JOIN oauth2_provider_apps
    ON oauth2_provider_apps.id = oauth2_provider_app_secrets.app_id
WHERE
  oauth2_provider_app_tokens.user_id = $1
GROUP BY
  oauth2_provider_apps.id;

-- name: DeleteOAuth2ProviderAppTokensByAppAndUserID :exec
DELETE FROM
  oauth2_provider_app_tokens
USING
  oauth2_provider_app_secrets
WHERE
  oauth2_provider_app_secrets.id = oauth2_provider_app_tokens.app_secret_id
  AND oauth2_provider_app_secrets.app_id = $1
  AND oauth2_provider_app_tokens.user_id = $2;

-- RFC 7591/7592 Dynamic Client Registration queries

-- name: GetOAuth2ProviderAppByClientID :one
SELECT * FROM oauth2_provider_apps WHERE id = $1;

-- name: UpdateOAuth2ProviderAppByClientID :one
UPDATE oauth2_provider_apps SET
    updated_at = $2,
    name = $3,
    icon = $4,
    redirect_uris = $5,
    client_type = $6,
    client_secret_expires_at = $7,
    grant_types = $8,
    response_types = $9,
    token_endpoint_auth_method = $10,
    scope = $11,
    contacts = $12,
    client_uri = $13,
    logo_uri = $14,
    tos_uri = $15,
    policy_uri = $16,
    jwks_uri = $17,
    jwks = $18,
    software_id = $19,
    software_version = $20
WHERE id = $1 RETURNING *;

-- name: DeleteOAuth2ProviderAppByClientID :exec
DELETE FROM oauth2_provider_apps WHERE id = $1;

-- name: GetOAuth2ProviderAppByRegistrationToken :one
SELECT * FROM oauth2_provider_apps WHERE registration_access_token = $1;

-- RFC 8628 Device Authorization Grant queries

-- name: InsertOAuth2ProviderDeviceCode :one
INSERT INTO oauth2_provider_device_codes (
    id,
    created_at,
    expires_at,
    device_code_hash,
    device_code_prefix,
    user_code,
    client_id,
    verification_uri,
    verification_uri_complete,
    scope,
    resource_uri,
    polling_interval
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    $10,
    $11,
    $12
) RETURNING *;

-- name: GetOAuth2ProviderDeviceCodeByPrefix :one
SELECT * FROM oauth2_provider_device_codes WHERE device_code_prefix = $1;

-- name: ConsumeOAuth2ProviderDeviceCodeByPrefix :one
DELETE FROM oauth2_provider_device_codes
WHERE device_code_prefix = $1 AND expires_at > NOW() AND status = 'authorized'
RETURNING *;

-- name: GetOAuth2ProviderDeviceCodeByUserCode :one
SELECT * FROM oauth2_provider_device_codes WHERE user_code = $1;

-- name: GetOAuth2ProviderDeviceCodeByID :one
SELECT * FROM oauth2_provider_device_codes WHERE id = $1;

-- name: UpdateOAuth2ProviderDeviceCodeAuthorization :one
UPDATE oauth2_provider_device_codes SET
    user_id = $2,
    status = $3
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: DeleteOAuth2ProviderDeviceCodeByID :exec
DELETE FROM oauth2_provider_device_codes WHERE id = $1;

-- name: DeleteExpiredOAuth2ProviderDeviceCodes :exec
DELETE FROM oauth2_provider_device_codes
WHERE expires_at < NOW();

-- name: DeleteExpiredOAuth2ProviderAppCodes :exec
DELETE FROM oauth2_provider_app_codes
WHERE expires_at < NOW();

-- name: DeleteExpiredOAuth2ProviderAppTokens :exec
DELETE FROM oauth2_provider_app_tokens
WHERE expires_at < NOW();

-- name: GetOAuth2ProviderDeviceCodesByClientID :many
SELECT * FROM oauth2_provider_device_codes
WHERE client_id = $1
ORDER BY created_at DESC;
