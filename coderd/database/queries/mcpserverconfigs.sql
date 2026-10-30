-- name: GetMCPServerConfigByID :one
SELECT
    *
FROM
    mcp_server_configs
WHERE
    id = @id::uuid;

-- name: GetMCPServerConfigByIDForUpdate :one
SELECT
    *
FROM
    mcp_server_configs
WHERE
    id = @id::uuid
FOR UPDATE;

-- name: GetMCPServerConfigByOrganizationAndSlug :one
SELECT
    *
FROM
    mcp_server_configs
WHERE
    organization_id = @organization_id::uuid
    AND slug = @slug::text;

-- name: GetMCPServerConfigsByOrganization :many
SELECT
    *
FROM
    mcp_server_configs
WHERE
    organization_id = @organization_id::uuid
    -- Authorize Filter clause will be injected below in GetAuthorizedMCPServerConfigs
    -- @authorize_filter
ORDER BY
    display_name ASC;

-- name: GetEnabledMCPServerConfigsByOrganization :many
SELECT
    *
FROM
    mcp_server_configs
WHERE
    organization_id = @organization_id::uuid
    AND enabled = TRUE
ORDER BY
    display_name ASC;

-- name: GetEnabledMCPServerConfigsByOrganizationAndIDs :many
SELECT
    *
FROM
    mcp_server_configs
WHERE
    organization_id = @organization_id::uuid
    AND id = ANY(@ids::uuid[])
    AND enabled = TRUE
ORDER BY
    display_name ASC;

-- name: GetForcedMCPServerConfigsByOrganization :many
SELECT
    *
FROM
    mcp_server_configs
WHERE
    organization_id = @organization_id::uuid
    AND enabled = TRUE
    AND availability = 'force_on'
ORDER BY
    display_name ASC;

-- name: InsertMCPServerConfig :one
INSERT INTO mcp_server_configs (
    id,
    organization_id,
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
    oauth2_revocation_url,
    oauth2_resource,
    oauth2_scopes,
    api_key_header,
    api_key_value,
    api_key_value_key_id,
    api_key_prefix,
    custom_headers,
    custom_headers_key_id,
    signing_secret,
    signing_secret_key_id,
    tool_allow_list,
    tool_deny_list,
    availability,
    enabled,
    model_intent,
    allow_in_plan_mode,
    forward_coder_headers,
    group_acl,
    user_acl,
    created_by,
    updated_by
) VALUES (
    @id::uuid,
    @organization_id::uuid,
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
    @oauth2_revocation_url::text,
    @oauth2_resource::text,
    @oauth2_scopes::text,
    @api_key_header::text,
    @api_key_value::text,
    sqlc.narg('api_key_value_key_id')::text,
    @api_key_prefix::text,
    @custom_headers::text,
    sqlc.narg('custom_headers_key_id')::text,
    @signing_secret::text,
    sqlc.narg('signing_secret_key_id')::text,
    @tool_allow_list::text[],
    @tool_deny_list::text[],
    @availability::text,
    @enabled::boolean,
    @model_intent::boolean,
    @allow_in_plan_mode::boolean,
    @forward_coder_headers::boolean,
    @group_acl,
    @user_acl,
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
    oauth2_revocation_url = @oauth2_revocation_url::text,
    oauth2_resource = @oauth2_resource::text,
    oauth2_scopes = @oauth2_scopes::text,
    api_key_header = @api_key_header::text,
    api_key_value = @api_key_value::text,
    api_key_value_key_id = sqlc.narg('api_key_value_key_id')::text,
    api_key_prefix = @api_key_prefix::text,
    custom_headers = @custom_headers::text,
    custom_headers_key_id = sqlc.narg('custom_headers_key_id')::text,
    signing_secret = @signing_secret::text,
    signing_secret_key_id = sqlc.narg('signing_secret_key_id')::text,
    tool_allow_list = @tool_allow_list::text[],
    tool_deny_list = @tool_deny_list::text[],
    availability = @availability::text,
    enabled = @enabled::boolean,
    model_intent = @model_intent::boolean,
    allow_in_plan_mode = @allow_in_plan_mode::boolean,
    forward_coder_headers = @forward_coder_headers::boolean,
    updated_by = @updated_by::uuid,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: UpdateMCPServerConfigACLByID :exec
UPDATE mcp_server_configs
SET
    group_acl = @group_acl,
    user_acl = @user_acl,
    updated_by = @updated_by::uuid,
    updated_at = NOW()
WHERE
    id = @id::uuid;

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

-- name: GetMCPServerUserTokenByID :one
SELECT
    *
FROM
    mcp_server_user_tokens
WHERE
    id = @id::uuid;

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
    -- New token material means the user re-authenticated, so any
    -- cached permanent refresh failure no longer applies.
    oauth_refresh_failure_reason = '',
    updated_at = NOW()
RETURNING
    *;

-- name: UpdateMCPServerUserTokenFromRefresh :one
-- Refresh persistence must not recreate a token deleted by disconnect.
-- The optimistic lock also prevents stale refreshes from replacing newer tokens.
UPDATE mcp_server_user_tokens
SET
    access_token = @access_token::text,
    access_token_key_id = sqlc.narg('access_token_key_id')::text,
    refresh_token = @refresh_token::text,
    refresh_token_key_id = sqlc.narg('refresh_token_key_id')::text,
    token_type = @token_type::text,
    expiry = sqlc.narg('expiry')::timestamptz,
    oauth_refresh_failure_reason = '',
    updated_at = NOW()
WHERE
    id = @id::uuid
    AND updated_at = @updated_at::timestamptz
RETURNING
    *;

-- name: MarkMCPServerUserTokenRefreshFailure :one
-- Records a permanent refresh failure (e.g. revoked grant) and clears
-- the dead token material so it is never attached to a request again.
-- The updated_at predicate provides optimistic concurrency: if another
-- request refreshed or replaced the token since it was read, this
-- update matches zero rows and returns sql.ErrNoRows.
UPDATE mcp_server_user_tokens
SET
    access_token = '',
    access_token_key_id = NULL,
    refresh_token = '',
    refresh_token_key_id = NULL,
    expiry = NULL,
    oauth_refresh_failure_reason = @oauth_refresh_failure_reason::text,
    updated_at = NOW()
WHERE
    id = @id::uuid
    AND updated_at = @updated_at::timestamptz
RETURNING
    *;

-- name: DeleteMCPServerUserToken :exec
DELETE FROM
    mcp_server_user_tokens
WHERE
    mcp_server_config_id = @mcp_server_config_id::uuid
    AND user_id = @user_id::uuid;

-- name: DeleteMCPServerUserTokensByConfigID :exec
DELETE FROM
    mcp_server_user_tokens
WHERE
    mcp_server_config_id = @mcp_server_config_id::uuid;

-- name: GetMCPServerGitHubAppCredential :one
SELECT
    *
FROM
    mcp_server_github_app_credentials
WHERE
    mcp_server_config_id = @mcp_server_config_id::uuid
    AND user_id = @user_id::uuid;

-- name: GetMCPServerGitHubAppCredentialsByUserID :many
SELECT
    *
FROM
    mcp_server_github_app_credentials
WHERE
    user_id = @user_id::uuid
ORDER BY
    updated_at DESC, mcp_server_config_id;

-- name: UpsertMCPServerGitHubAppCredential :one
INSERT INTO mcp_server_github_app_credentials (
    mcp_server_config_id,
    user_id,
    app_id,
    installation_id,
    private_key,
    private_key_key_id,
    repositories,
    use_for_git
) VALUES (
    @mcp_server_config_id::uuid,
    @user_id::uuid,
    @app_id::bigint,
    @installation_id::bigint,
    @private_key::text,
    sqlc.narg('private_key_key_id')::text,
    @repositories::text[],
    @use_for_git::boolean
)
ON CONFLICT (mcp_server_config_id, user_id) DO UPDATE SET
    app_id = @app_id::bigint,
    installation_id = @installation_id::bigint,
    private_key = @private_key::text,
    private_key_key_id = sqlc.narg('private_key_key_id')::text,
    repositories = @repositories::text[],
    use_for_git = @use_for_git::boolean,
    updated_at = NOW()
RETURNING
    *;

-- name: DeleteMCPServerGitHubAppCredential :exec
DELETE FROM
    mcp_server_github_app_credentials
WHERE
    mcp_server_config_id = @mcp_server_config_id::uuid
    AND user_id = @user_id::uuid;

-- name: DeleteMCPServerGitHubAppCredentialsByConfigID :exec
DELETE FROM
    mcp_server_github_app_credentials
WHERE
    mcp_server_config_id = @mcp_server_config_id::uuid;

-- name: CleanupDeletedMCPServerIDsFromChats :exec
UPDATE chats
SET mcp_server_ids = (
    SELECT COALESCE(array_agg(sid), '{}')
    FROM unnest(chats.mcp_server_ids) AS sid
    WHERE sid IN (SELECT id FROM mcp_server_configs)
)
WHERE mcp_server_ids != '{}'
  AND NOT (mcp_server_ids <@ COALESCE((SELECT array_agg(id) FROM mcp_server_configs), '{}'));
