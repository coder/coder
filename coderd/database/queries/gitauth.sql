-- name: GetGitAuthLink :one
SELECT * FROM git_auth_links WHERE provider_id = $1 AND user_id = $2;

-- name: InsertGitAuthLink :one
INSERT INTO git_auth_links (
    provider_id,
    user_id,
    created_at,
    updated_at,
    oauth_access_token,
    oauth_refresh_token,
    oauth_expiry
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7
) RETURNING *;

-- name: UpdateGitAuthLink :one
UPDATE git_auth_links SET
    updated_at = $3,
    oauth_access_token = $4,
    oauth_refresh_token = $5,
    oauth_expiry = $6
WHERE provider_id = $1 AND user_id = $2 RETURNING *;
