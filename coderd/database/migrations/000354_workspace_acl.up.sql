DROP VIEW workspaces_expanded;

ALTER TABLE workspaces
	ADD COLUMN group_acl jsonb not null default '{}'::jsonb,
	ADD COLUMN user_acl  jsonb not null default '{}'::jsonb;

-- Recreate the view, now including the new columns
CREATE VIEW workspaces_expanded AS
	SELECT workspaces.id,
		workspaces.created_at,
		workspaces.updated_at,
		workspaces.owner_id,
		workspaces.organization_id,
		workspaces.template_id,
		workspaces.deleted,
		workspaces.name,
		workspaces.autostart_schedule,
		workspaces.ttl,
		workspaces.last_used_at,
		workspaces.dormant_at,
		workspaces.deleting_at,
		workspaces.automatic_updates,
		workspaces.favorite,
		workspaces.next_start_at,
		workspaces.group_acl,
		workspaces.user_acl,
		visible_users.avatar_url AS owner_avatar_url,
		visible_users.username AS owner_username,
		visible_users.name AS owner_name,
		organizations.name AS organization_name,
		organizations.display_name AS organization_display_name,
		organizations.icon AS organization_icon,
		organizations.description AS organization_description,
		templates.name AS template_name,
		templates.display_name AS template_display_name,
		templates.icon AS template_icon,
		templates.description AS template_description
	FROM (((workspaces
		JOIN visible_users ON ((workspaces.owner_id = visible_users.id)))
		JOIN organizations ON ((workspaces.organization_id = organizations.id)))
		JOIN templates ON ((workspaces.template_id = templates.id)));

COMMENT ON VIEW workspaces_expanded IS 'Joins in the display name information such as username, avatar, and organization name.';
