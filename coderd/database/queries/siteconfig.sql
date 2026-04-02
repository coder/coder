-- name: UpsertDefaultProxy :exec
-- The default proxy is implied and not actually stored in the database.
-- So we need to store it's configuration here for display purposes.
-- The functional values are immutable and controlled implicitly.
INSERT INTO site_configs (key, value)
VALUES
    ('default_proxy_display_name', @display_name :: text),
    ('default_proxy_icon_url', @icon_url :: text)
ON CONFLICT
    (key)
DO UPDATE SET value = EXCLUDED.value WHERE site_configs.key = EXCLUDED.key
;

-- name: GetDefaultProxyConfig :one
SELECT
	COALESCE((SELECT value FROM site_configs WHERE key = 'default_proxy_display_name'), 'Default') :: text AS display_name,
	COALESCE((SELECT value FROM site_configs WHERE key = 'default_proxy_icon_url'), '/emojis/1f3e1.png') :: text AS icon_url
;

-- name: InsertDeploymentID :exec
INSERT INTO site_configs (key, value) VALUES ('deployment_id', $1);

-- name: GetDeploymentID :one
SELECT value FROM site_configs WHERE key = 'deployment_id';

-- name: InsertDERPMeshKey :exec
INSERT INTO site_configs (key, value) VALUES ('derp_mesh_key', $1);

-- name: GetDERPMeshKey :one
SELECT value FROM site_configs WHERE key = 'derp_mesh_key';

-- name: UpsertLastUpdateCheck :exec
INSERT INTO site_configs (key, value) VALUES ('last_update_check', $1)
ON CONFLICT (key) DO UPDATE SET value = $1 WHERE site_configs.key = 'last_update_check';

-- name: GetLastUpdateCheck :one
SELECT value FROM site_configs WHERE key = 'last_update_check';

-- name: UpsertAnnouncementBanners :exec
INSERT INTO site_configs (key, value) VALUES ('announcement_banners', $1)
ON CONFLICT (key) DO UPDATE SET value = $1 WHERE site_configs.key = 'announcement_banners';

-- name: GetAnnouncementBanners :one
SELECT value FROM site_configs WHERE key = 'announcement_banners';

-- name: UpsertLogoURL :exec
INSERT INTO site_configs (key, value) VALUES ('logo_url', $1)
ON CONFLICT (key) DO UPDATE SET value = $1 WHERE site_configs.key = 'logo_url';

-- name: GetLogoURL :one
SELECT value FROM site_configs WHERE key = 'logo_url';

-- name: UpsertApplicationName :exec
INSERT INTO site_configs (key, value) VALUES ('application_name', $1)
ON CONFLICT (key) DO UPDATE SET value = $1 WHERE site_configs.key = 'application_name';

-- name: GetApplicationName :one
SELECT value FROM site_configs WHERE key = 'application_name';

-- name: GetHealthSettings :one
SELECT
	COALESCE((SELECT value FROM site_configs WHERE key = 'health_settings'), '{}') :: text AS health_settings
;

-- name: UpsertHealthSettings :exec
INSERT INTO site_configs (key, value) VALUES ('health_settings', $1)
ON CONFLICT (key) DO UPDATE SET value = $1 WHERE site_configs.key = 'health_settings';

-- name: GetNotificationsSettings :one
SELECT
	COALESCE((SELECT value FROM site_configs WHERE key = 'notifications_settings'), '{}') :: text AS notifications_settings
;

-- name: UpsertNotificationsSettings :exec
INSERT INTO site_configs (key, value) VALUES ('notifications_settings', $1)
ON CONFLICT (key) DO UPDATE SET value = $1 WHERE site_configs.key = 'notifications_settings';

-- name: GetPrebuildsSettings :one
SELECT
	COALESCE((SELECT value FROM site_configs WHERE key = 'prebuilds_settings'), '{}') :: text AS prebuilds_settings
;

-- name: UpsertPrebuildsSettings :exec
INSERT INTO site_configs (key, value) VALUES ('prebuilds_settings', $1)
ON CONFLICT (key) DO UPDATE SET value = $1 WHERE site_configs.key = 'prebuilds_settings';

-- name: GetRuntimeConfig :one
SELECT value FROM site_configs WHERE site_configs.key = $1;

-- name: UpsertRuntimeConfig :exec
INSERT INTO site_configs (key, value) VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = $2 WHERE site_configs.key = $1;

-- name: DeleteRuntimeConfig :exec
DELETE FROM site_configs
WHERE site_configs.key = $1;

-- name: GetOAuth2GithubDefaultEligible :one
SELECT
	CASE
		WHEN value = 'true' THEN TRUE
		ELSE FALSE
	END
FROM site_configs
WHERE key = 'oauth2_github_default_eligible';

-- name: UpsertOAuth2GithubDefaultEligible :exec
INSERT INTO site_configs (key, value)
VALUES (
    'oauth2_github_default_eligible',
    CASE
        WHEN sqlc.arg(eligible)::bool THEN 'true'
        ELSE 'false'
    END
)
ON CONFLICT (key) DO UPDATE
SET value = CASE
    WHEN sqlc.arg(eligible)::bool THEN 'true'
    ELSE 'false'
END
WHERE site_configs.key = 'oauth2_github_default_eligible';

-- name: UpsertWebpushVAPIDKeys :exec
INSERT INTO site_configs (key, value)
VALUES
    ('webpush_vapid_public_key', @vapid_public_key :: text),
    ('webpush_vapid_private_key', @vapid_private_key :: text)
ON CONFLICT (key)
DO UPDATE SET value = EXCLUDED.value WHERE site_configs.key = EXCLUDED.key;

-- name: GetWebpushVAPIDKeys :one
SELECT
    COALESCE((SELECT value FROM site_configs WHERE key = 'webpush_vapid_public_key'), '') :: text AS vapid_public_key,
    COALESCE((SELECT value FROM site_configs WHERE key = 'webpush_vapid_private_key'), '') :: text AS vapid_private_key;

-- name: GetChatSystemPrompt :one
SELECT
	COALESCE((SELECT value FROM site_configs WHERE key = 'agents_chat_system_prompt'), '') :: text AS chat_system_prompt;

-- GetChatSystemPromptConfig returns both chat system prompt settings in a
-- single read to avoid torn reads between separate site-config lookups.
-- The include-default fallback preserves the legacy behavior where a
-- non-empty custom prompt implied opting out before the explicit toggle
-- existed.
-- name: GetChatSystemPromptConfig :one
SELECT
    COALESCE((SELECT value FROM site_configs WHERE key = 'agents_chat_system_prompt'), '') :: text AS chat_system_prompt,
    COALESCE(
        (SELECT value = 'true' FROM site_configs WHERE key = 'agents_chat_include_default_system_prompt'),
        NOT EXISTS (
            SELECT 1
            FROM site_configs
            WHERE key = 'agents_chat_system_prompt'
                AND value != ''
        )
    ) :: boolean AS include_default_system_prompt;

-- name: UpsertChatSystemPrompt :exec
INSERT INTO site_configs (key, value) VALUES ('agents_chat_system_prompt', $1)
ON CONFLICT (key) DO UPDATE SET value = $1 WHERE site_configs.key = 'agents_chat_system_prompt';

-- name: GetChatDesktopEnabled :one
SELECT
	COALESCE((SELECT value = 'true' FROM site_configs WHERE key = 'agents_desktop_enabled'), false) :: boolean AS enable_desktop;

-- name: UpsertChatDesktopEnabled :exec
INSERT INTO site_configs (key, value)
VALUES (
    'agents_desktop_enabled',
    CASE
        WHEN sqlc.arg(enable_desktop)::bool THEN 'true'
        ELSE 'false'
    END
)
ON CONFLICT (key) DO UPDATE
SET value = CASE
    WHEN sqlc.arg(enable_desktop)::bool THEN 'true'
    ELSE 'false'
END
WHERE site_configs.key = 'agents_desktop_enabled';

-- GetChatTemplateAllowlist returns the JSON-encoded template allowlist.
-- Returns an empty string when no allowlist has been configured (all templates allowed).
-- name: GetChatTemplateAllowlist :one
SELECT
	COALESCE((SELECT value FROM site_configs WHERE key = 'agents_template_allowlist'), '') :: text AS template_allowlist;

-- GetChatIncludeDefaultSystemPrompt preserves the legacy default
-- for deployments created before the explicit include-default toggle.
-- When the toggle is unset, a non-empty custom prompt implies false;
-- otherwise the setting defaults to true.
-- name: GetChatIncludeDefaultSystemPrompt :one
SELECT
    COALESCE(
        (SELECT value = 'true' FROM site_configs WHERE key = 'agents_chat_include_default_system_prompt'),
        NOT EXISTS (
            SELECT 1
            FROM site_configs
            WHERE key = 'agents_chat_system_prompt'
                AND value != ''
        )
    ) :: boolean AS include_default_system_prompt;

-- name: UpsertChatIncludeDefaultSystemPrompt :exec
INSERT INTO site_configs (key, value)
VALUES (
    'agents_chat_include_default_system_prompt',
    CASE
        WHEN sqlc.arg(include_default_system_prompt)::bool THEN 'true'
        ELSE 'false'
    END
)
ON CONFLICT (key) DO UPDATE
SET value = CASE
    WHEN sqlc.arg(include_default_system_prompt)::bool THEN 'true'
    ELSE 'false'
END
WHERE site_configs.key = 'agents_chat_include_default_system_prompt';

-- name: GetChatWorkspaceTTL :one
-- Returns the global TTL for chat workspaces as a Go duration string.
-- Returns "0s" (disabled) when no value has been configured.
SELECT
    COALESCE(
        (SELECT value FROM site_configs WHERE key = 'agents_workspace_ttl'),
        '0s'
    )::text AS workspace_ttl;

-- name: UpsertChatTemplateAllowlist :exec
INSERT INTO site_configs (key, value) VALUES ('agents_template_allowlist', @template_allowlist)
ON CONFLICT (key) DO UPDATE SET value = @template_allowlist WHERE site_configs.key = 'agents_template_allowlist';

-- name: UpsertChatWorkspaceTTL :exec
INSERT INTO site_configs (key, value)
VALUES ('agents_workspace_ttl', @workspace_ttl::text)
ON CONFLICT (key) DO UPDATE
SET value = @workspace_ttl::text
WHERE site_configs.key = 'agents_workspace_ttl';

-- name: GetChatRetentionDays :one
-- Returns the chat retention period in days. Chats archived longer
-- than this and orphaned chat files older than this are purged by
-- dbpurge. Returns 30 (days) when no value has been configured.
-- A value of 0 disables chat purging entirely.
SELECT COALESCE(
    (SELECT value::integer FROM site_configs
     WHERE key = 'agents_chat_retention_days'),
    30
) :: integer AS retention_days;

-- name: UpsertChatRetentionDays :exec
INSERT INTO site_configs (key, value)
VALUES ('agents_chat_retention_days', CAST(@retention_days AS integer)::text)
ON CONFLICT (key) DO UPDATE SET value = CAST(@retention_days AS integer)::text
WHERE site_configs.key = 'agents_chat_retention_days';
