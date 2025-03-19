-- Do the dance of dropping a view...
DROP VIEW template_version_with_user;

-- ...removing the column we added to the table...
alter table "template_versions" drop column "import_graph";

-- ...and finally recreating the view.
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
