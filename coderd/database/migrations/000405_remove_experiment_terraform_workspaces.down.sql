DROP VIEW template_with_names;
ALTER TABLE templates ADD COLUMN use_terraform_workspace_cache BOOL NOT NULL DEFAULT false;

CREATE VIEW template_with_names AS
SELECT templates.*,
	   COALESCE(visible_users.avatar_url, ''::text) AS created_by_avatar_url,
	   COALESCE(visible_users.username, ''::text) AS created_by_username,
	   COALESCE(visible_users.name, ''::text) AS created_by_name,
	   COALESCE(organizations.name, ''::text) AS organization_name,
	   COALESCE(organizations.display_name, ''::text) AS organization_display_name,
	   COALESCE(organizations.icon, ''::text) AS organization_icon
FROM ((templates
	LEFT JOIN visible_users ON ((templates.created_by = visible_users.id)))
	LEFT JOIN organizations ON ((templates.organization_id = organizations.id)));

COMMENT ON VIEW template_with_names IS 'Joins in the display name information such as username, avatar, and organization name.';
