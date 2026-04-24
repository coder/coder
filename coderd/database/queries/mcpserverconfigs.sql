-- name: GetMCPServerConfigByID :one
SELECT
    *
FROM
    mcp_server_configs
WHERE
    id = @id::uuid;

-- name: GetMCPServerConfigBySlug :one
SELECT
    *
FROM
    mcp_server_configs
WHERE
    slug = @slug::text;

-- name: GetMCPServerConfigs :many
SELECT
    *
FROM
    mcp_server_configs
ORDER BY
    display_name ASC;

-- name: GetEnabledMCPServerConfigs :many
SELECT
    *
FROM
    mcp_server_configs
WHERE
    enabled = TRUE
ORDER BY
    display_name ASC;

-- name: GetMCPServerConfigsByIDs :many
SELECT
    *
FROM
    mcp_server_configs
WHERE
    id = ANY(@ids::uuid[])
ORDER BY
    display_name ASC;

-- name: GetForcedMCPServerConfigs :many
SELECT
    *
FROM
    mcp_server_configs
WHERE
    enabled = TRUE
    AND availability = 'force_on'
ORDER BY
    display_name ASC;

-- name: InsertMCPServerConfig :one
INSERT INTO mcp_server_configs (
    display_name,
    slug,
    description,
    icon_url,
    transport,
    url,
    auth_type,
    oauth2_client_id,
    oauth2_client_secret,
    oauth2_client_secret_key_id,
    oauth2_auth_url,
    oauth2_token_url,
    oauth2_scopes,
    api_key_header,
    api_key_value,
    api_key_value_key_id,
    custom_headers,
    custom_headers_key_id,
    tool_allow_list,
    tool_deny_list,
    availability,
    enabled,
    model_intent,
    allow_in_plan_mode,
    created_by,
    updated_by
) VALUES (
    @display_name::text,
    @slug::text,
    @description::text,
    @icon_url::text,
    @transport::text,
    @url::text,
    @auth_type::text,
    @oauth2_client_id::text,
    @oauth2_client_secret::text,
    sqlc.narg('oauth2_client_secret_key_id')::text,
    @oauth2_auth_url::text,
    @oauth2_token_url::text,
    @oauth2_scopes::text,
    @api_key_header::text,
    @api_key_value::text,
    sqlc.narg('api_key_value_key_id')::text,
    @custom_headers::text,
    sqlc.narg('custom_headers_key_id')::text,
    @tool_allow_list::text[],
    @tool_deny_list::text[],
    @availability::text,
    @enabled::boolean,
    @model_intent::boolean,
    @allow_in_plan_mode::boolean,
    @created_by::uuid,
    @updated_by::uuid
)
RETURNING
    *;

-- name: UpdateMCPServerConfig :one
UPDATE
    mcp_server_configs
SET
    display_name = @display_name::text,
    slug = @slug::text,
    description = @description::text,
    icon_url = @icon_url::text,
    transport = @transport::text,
    url = @url::text,
    auth_type = @auth_type::text,
    oauth2_client_id = @oauth2_client_id::text,
    oauth2_client_secret = @oauth2_client_secret::text,
    oauth2_client_secret_key_id = sqlc.narg('oauth2_client_secret_key_id')::text,
    oauth2_auth_url = @oauth2_auth_url::text,
    oauth2_token_url = @oauth2_token_url::text,
    oauth2_scopes = @oauth2_scopes::text,
    api_key_header = @api_key_header::text,
    api_key_value = @api_key_value::text,
    api_key_value_key_id = sqlc.narg('api_key_value_key_id')::text,
    custom_headers = @custom_headers::text,
    custom_headers_key_id = sqlc.narg('custom_headers_key_id')::text,
    tool_allow_list = @tool_allow_list::text[],
    tool_deny_list = @tool_deny_list::text[],
    availability = @availability::text,
    enabled = @enabled::boolean,
    model_intent = @model_intent::boolean,
    allow_in_plan_mode = @allow_in_plan_mode::boolean,
    updated_by = @updated_by::uuid,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: DeleteMCPServerConfigByID :exec
DELETE FROM
    mcp_server_configs
WHERE
    id = @id::uuid;

-- name: GetMCPServerUserToken :one
SELECT
    *
FROM
    mcp_server_user_tokens
WHERE
    mcp_server_config_id = @mcp_server_config_id::uuid
    AND user_id = @user_id::uuid;

-- name: GetMCPServerUserTokensByUserID :many
SELECT
    *
FROM
    mcp_server_user_tokens
WHERE
    user_id = @user_id::uuid;

-- name: UpsertMCPServerUserToken :one
INSERT INTO mcp_server_user_tokens (
    mcp_server_config_id,
    user_id,
    access_token,
    access_token_key_id,
    refresh_token,
    refresh_token_key_id,
    token_type,
    expiry
) VALUES (
    @mcp_server_config_id::uuid,
    @user_id::uuid,
    @access_token::text,
    sqlc.narg('access_token_key_id')::text,
    @refresh_token::text,
    sqlc.narg('refresh_token_key_id')::text,
    @token_type::text,
    sqlc.narg('expiry')::timestamptz
)
ON CONFLICT (mcp_server_config_id, user_id) DO UPDATE SET
    access_token = @access_token::text,
    access_token_key_id = sqlc.narg('access_token_key_id')::text,
    refresh_token = @refresh_token::text,
    refresh_token_key_id = sqlc.narg('refresh_token_key_id')::text,
    token_type = @token_type::text,
    expiry = sqlc.narg('expiry')::timestamptz,
    updated_at = NOW()
RETURNING
    *;

-- name: DeleteMCPServerUserToken :exec
DELETE FROM
    mcp_server_user_tokens
WHERE
    mcp_server_config_id = @mcp_server_config_id::uuid
    AND user_id = @user_id::uuid;

-- name: CleanupDeletedMCPServerIDsFromChats :exec
UPDATE chats
SET mcp_server_ids = (
    SELECT COALESCE(array_agg(sid), '{}')
    FROM unnest(chats.mcp_server_ids) AS sid
    WHERE sid IN (SELECT id FROM mcp_server_configs)
)
WHERE mcp_server_ids != '{}'
  AND NOT (mcp_server_ids <@ COALESCE((SELECT array_agg(id) FROM mcp_server_configs), '{}'));
