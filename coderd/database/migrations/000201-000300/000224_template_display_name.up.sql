-- Update the template_with_names view by recreating it.
DROP VIEW template_with_names;
CREATE VIEW
	template_with_names
AS
SELECT
	templates.*,
	coalesce(visible_users.avatar_url, '') AS created_by_avatar_url,
	coalesce(visible_users.username, '') AS created_by_username,
	coalesce(organizations.name, '') AS organization_name,
	coalesce(organizations.display_name, '') AS organization_display_name,
	coalesce(organizations.icon, '') AS organization_icon
FROM
	templates
		LEFT JOIN
	visible_users
	ON
		templates.created_by = visible_users.id
		LEFT JOIN
	organizations
	ON templates.organization_id = organizations.id
;

COMMENT ON VIEW template_with_names IS 'Joins in the display name information such as username, avatar, and organization name.';
