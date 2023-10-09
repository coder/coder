BEGIN;

ALTER TABLE template_versions RENAME COLUMN git_auth_providers TO external_auth_providers;

ALTER TABLE git_auth_links RENAME TO external_auth_links;

DROP VIEW template_version_with_user;
-- If you need to update this view, put 'DROP VIEW template_version_with_user;' before this.
CREATE VIEW
	template_version_with_user
AS
SELECT
	template_versions.*,
	coalesce(visible_users.avatar_url, '') AS created_by_avatar_url,
	coalesce(visible_users.username, '') AS created_by_username
FROM
	template_versions
	LEFT JOIN
		visible_users
	ON
		template_versions.created_by = visible_users.id;

COMMENT ON VIEW template_version_with_user IS 'Joins in the username + avatar url of the created by user.';

COMMENT ON COLUMN template_versions.external_auth_providers IS 'IDs of External auth providers for a specific template version';

COMMIT;
