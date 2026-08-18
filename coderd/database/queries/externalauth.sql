-- name: GetExternalAuthLink :one
SELECT * FROM external_auth_links WHERE provider_id = $1 AND user_id = $2;

-- name: DeleteExternalAuthLink :exec
DELETE FROM external_auth_links WHERE provider_id = $1 AND user_id = $2;

-- name: GetExternalAuthLinksByUserID :many
SELECT * FROM external_auth_links WHERE user_id = $1;

-- name: InsertExternalAuthLink :one
INSERT INTO external_auth_links (
    provider_id,
    user_id,
    created_at,
    updated_at,
    oauth_access_token,
    oauth_access_token_key_id,
    oauth_refresh_token,
    oauth_refresh_token_key_id,
    oauth_expiry,
	oauth_extra
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
	$10
) RETURNING *;

-- name: UpdateExternalAuthLink :one
-- If a refresh lease is provided, the row is only updated if the lease matches.
UPDATE external_auth_links SET
	updated_at = $4,
	oauth_access_token = $5,
	oauth_access_token_key_id = $6,
	oauth_refresh_token = $7,
	oauth_refresh_token_key_id = $8,
	oauth_expiry = $9,
	oauth_extra = $10,
	oauth_refresh_failure_reason = $11
WHERE
	provider_id = $1
	AND user_id = $2
	AND (refresh_lease_expires_at = $3 OR $3 IS NULL)
RETURNING *;

-- name: SetExternalAuthLinkRefreshLease :exec
-- If an old lease is set, the row will be only updated if it matches the
-- current lease.
UPDATE
	external_auth_links
SET
	refresh_lease_expires_at = @refresh_lease_expires_at
WHERE
	provider_id = @provider_id
	AND user_id = @user_id
	AND (refresh_lease_expires_at = @old_refresh_lease_expires_at OR @old_refresh_lease_expires_at IS NULL);
