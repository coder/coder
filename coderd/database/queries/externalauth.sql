-- name: GetExternalAuthLink :one
SELECT * FROM external_auth_links WHERE provider_id = $1 AND user_id = $2;

-- name: DeleteExternalAuthLink :exec
DELETE FROM external_auth_links WHERE provider_id = $1 AND user_id = $2;

-- name: GetExternalAuthLinksByUserID :many
SELECT * FROM external_auth_links WHERE user_id = $1;

-- name: GetUsersByExternalAuthProviderUserID :many
-- Returns every Coder user whose external auth link for the provider
-- stores the given provider-side user id in oauth_extra (e.g. Slack's
-- authed_user.id). All matches are returned, including deleted or
-- suspended users, so callers can detect ambiguous or unusable
-- identity mappings instead of silently picking one account.
SELECT users.*
FROM external_auth_links
JOIN users ON users.id = external_auth_links.user_id
WHERE external_auth_links.provider_id = @provider_id::text
    AND external_auth_links.oauth_extra -> 'authed_user' ->> 'id' = @external_user_id::text
ORDER BY users.created_at ASC, users.id ASC;

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
UPDATE external_auth_links SET
    updated_at = $3,
    oauth_access_token = $4,
    oauth_access_token_key_id = $5,
    oauth_refresh_token = $6,
    oauth_refresh_token_key_id = $7,
    oauth_expiry = $8,
	oauth_extra = $9,
	-- Only 'UpdateExternalAuthLinkRefreshToken' supports updating the oauth_refresh_failure_reason.
	-- Any updates to the external auth link, will be assumed to change the state and clear
	-- any cached errors.
	oauth_refresh_failure_reason = ''
WHERE provider_id = $1 AND user_id = $2 RETURNING *;

-- name: UpdateExternalAuthLinkRefreshToken :exec
-- Optimistic lock: only update the row if the refresh token in the database
-- still matches the one we read before attempting the refresh. This prevents
-- a concurrent caller that lost a token-refresh race from overwriting a valid
-- token stored by the winner.
UPDATE
	external_auth_links
SET
	-- oauth_refresh_failure_reason can be set to cache the failure reason
	-- for subsequent refresh attempts.
	oauth_refresh_failure_reason = @oauth_refresh_failure_reason,
	oauth_refresh_token = @oauth_refresh_token,
	updated_at = @updated_at
WHERE
    provider_id = @provider_id
AND
    user_id = @user_id
AND
    oauth_refresh_token = @old_oauth_refresh_token
AND
    -- Required for sqlc to generate a parameter for the oauth_refresh_token_key_id
    @oauth_refresh_token_key_id :: text = @oauth_refresh_token_key_id :: text;
