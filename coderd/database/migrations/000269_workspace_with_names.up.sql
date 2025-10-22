CREATE VIEW
	workspaces_expanded
AS
SELECT
	workspaces.*,
	-- Owner
	visible_users.avatar_url AS owner_avatar_url,
	visible_users.username AS owner_username,
	-- Organization
	organizations.name AS organization_name,
	organizations.display_name AS organization_display_name,
	organizations.icon AS organization_icon,
	organizations.description AS organization_description,
    -- Template
	templates.name AS template_name,
	templates.display_name AS template_display_name,
	templates.icon AS template_icon,
	templates.description AS template_description
FROM
	workspaces
	INNER JOIN
		visible_users
	ON
		workspaces.owner_id = visible_users.id
	INNER JOIN
		organizations
	ON workspaces.organization_id = organizations.id
	INNER JOIN
		templates
	ON workspaces.template_id = templates.id
;

COMMENT ON VIEW workspaces_expanded IS 'Joins in the display name information such as username, avatar, and organization name.';
