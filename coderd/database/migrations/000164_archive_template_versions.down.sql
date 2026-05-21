-- The view will be rebuilt with the new column
DROP VIEW template_version_with_user;

ALTER TABLE template_versions
	DROP COLUMN archived;

-- Restore the old version of the template_version_with_user view.
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
