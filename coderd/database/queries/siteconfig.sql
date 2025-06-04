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

-- name: GetAppSecurityKey :one
SELECT value FROM site_configs WHERE key = 'app_signing_key';

-- name: UpsertAppSecurityKey :exec
INSERT INTO site_configs (key, value) VALUES ('app_signing_key', $1)
ON CONFLICT (key) DO UPDATE set value = $1 WHERE site_configs.key = 'app_signing_key';

-- name: GetOAuthSigningKey :one
SELECT value FROM site_configs WHERE key = 'oauth_signing_key';

-- name: UpsertOAuthSigningKey :exec
INSERT INTO site_configs (key, value) VALUES ('oauth_signing_key', $1)
ON CONFLICT (key) DO UPDATE set value = $1 WHERE site_configs.key = 'oauth_signing_key';

-- name: GetCoordinatorResumeTokenSigningKey :one
SELECT value FROM site_configs WHERE key = 'coordinator_resume_token_signing_key';

-- name: UpsertCoordinatorResumeTokenSigningKey :exec
INSERT INTO site_configs (key, value) VALUES ('coordinator_resume_token_signing_key', $1)
ON CONFLICT (key) DO UPDATE set value = $1 WHERE site_configs.key = 'coordinator_resume_token_signing_key';

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
