CREATE VIEW
	workspaces_expanded
AS
SELECT
	workspaces.*,
	-- Owner
	coalesce(visible_users.avatar_url, '') AS owner_avatar_url,
	coalesce(visible_users.username, '') AS owner_username,
	-- Organization
	coalesce(organizations.name, '') AS organization_name,
	coalesce(organizations.display_name, '') AS organization_display_name,
	coalesce(organizations.icon, '') AS organization_icon,
	coalesce(organizations.description, '') AS organization_description,
    -- Template
	coalesce(templates.name, '') AS template_name,
	coalesce(templates.display_name, '') AS template_display_name,
	coalesce(templates.icon, '') AS template_icon,
	coalesce(templates.description, '') AS template_description
FROM
	workspaces
	LEFT JOIN
		visible_users
	ON
		workspaces.owner_id = visible_users.id
	LEFT JOIN
		organizations
	ON workspaces.organization_id = organizations.id
	LEFT JOIN
		templates
	ON workspaces.template_id = templates.id
;

COMMENT ON VIEW workspaces_expanded IS 'Joins in the display name information such as username, avatar, and organization name.';
