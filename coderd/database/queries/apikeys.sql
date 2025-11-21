-- name: GetAPIKeyByID :one
SELECT
	*
FROM
	api_keys
WHERE
	id = $1
LIMIT
	1;

-- name: GetAPIKeyByName :one
SELECT
	*
FROM
	api_keys
WHERE
	user_id = @user_id AND
	token_name = @token_name AND
-- there is no unique constraint on empty token names
	token_name != ''
LIMIT
	1;

-- name: GetAPIKeysLastUsedAfter :many
SELECT * FROM api_keys WHERE last_used > $1;

-- name: GetAPIKeysByLoginType :many
SELECT * FROM api_keys WHERE login_type = $1;

-- name: GetAPIKeysByUserID :many
SELECT * FROM api_keys WHERE login_type = $1 AND user_id = $2;

-- name: InsertAPIKey :one
INSERT INTO
	api_keys (
		id,
		lifetime_seconds,
		hashed_secret,
		ip_address,
		user_id,
		last_used,
		expires_at,
		created_at,
		updated_at,
		login_type,
		scopes,
		allow_list,
		token_name
	)
VALUES
	(@id,
	 -- If the lifetime is set to 0, default to 24hrs
	 CASE @lifetime_seconds::bigint
	     WHEN 0 THEN 86400
		 ELSE @lifetime_seconds::bigint
	 END
	 , @hashed_secret, @ip_address, @user_id, @last_used, @expires_at, @created_at, @updated_at, @login_type, @scopes, @allow_list, @token_name) RETURNING *;

-- name: UpdateAPIKeyByID :exec
UPDATE
	api_keys
SET
	last_used = $2,
	expires_at = $3,
	ip_address = $4
WHERE
	id = $1;

-- name: DeleteAPIKeyByID :exec
DELETE FROM
	api_keys
WHERE
	id = $1;

-- name: DeleteApplicationConnectAPIKeysByUserID :exec
DELETE FROM
	api_keys
WHERE
	user_id = $1 AND
	'coder:application_connect'::api_key_scope = ANY(scopes);

-- name: DeleteAPIKeysByUserID :exec
DELETE FROM
	api_keys
WHERE
	user_id = $1;

-- name: DeleteExpiredAPIKeys :exec
WITH expired_keys AS (
	SELECT id
	FROM api_keys
	-- expired keys only
	WHERE expires_at < @now::timestamptz
	LIMIT @limit_count
)
DELETE FROM
	api_keys
USING
	expired_keys
WHERE
	api_keys.id = expired_keys.id
;

-- name: ExpirePrebuildsAPIKeys :exec
-- Firstly, collect api_keys owned by the prebuilds user that correlate
-- to workspaces no longer owned by the prebuilds user.
WITH unexpired_prebuilds_workspace_session_tokens AS (
	SELECT id, SUBSTRING(token_name FROM 38 FOR 36)::uuid AS workspace_id
	FROM api_keys
	WHERE user_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0'::uuid
	AND expires_at > @now::timestamptz
	AND token_name SIMILAR TO 'c42fdf75-3097-471c-8c33-fb52454d81c0_[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}_session_token'
),
stale_prebuilds_workspace_session_tokens AS (
	SELECT upwst.id
	FROM unexpired_prebuilds_workspace_session_tokens upwst
	LEFT JOIN workspaces w
	ON w.id = upwst.workspace_id
	WHERE w.owner_id <> 'c42fdf75-3097-471c-8c33-fb52454d81c0'::uuid
),
-- Next, collect api_keys that belong to the prebuilds user but have no token name.
-- These were most likely created via 'coder login' as the prebuilds user.
unnamed_prebuilds_api_keys AS (
	SELECT id
	FROM api_keys
	WHERE user_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0'::uuid
	AND token_name = ''
	AND expires_at > @now::timestamptz
)
UPDATE api_keys
SET expires_at = @now::timestamptz
WHERE id IN (
	SELECT id FROM stale_prebuilds_workspace_session_tokens
	UNION
	SELECT id FROM unnamed_prebuilds_api_keys
);
