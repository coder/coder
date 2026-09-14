-- If you need to update this view, put 'DROP VIEW workspace_build_with_user;' before this.
CREATE VIEW
	workspace_build_with_user
AS
SELECT
	workspace_builds.*,
	coalesce(visible_users.avatar_url, '') AS initiator_by_avatar_url,
	coalesce(visible_users.username, '') AS initiator_by_username
FROM
	workspace_builds
	LEFT JOIN
		visible_users
	ON
		workspace_builds.initiator_id = visible_users.id;

COMMENT ON VIEW workspace_build_with_user IS 'Joins in the username + avatar url of the initiated by user.';


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
