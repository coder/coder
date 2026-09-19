DROP VIEW template_with_names;

ALTER TABLE templates ADD COLUMN exit_node_id uuid REFERENCES exit_nodes(id) ON DELETE SET NULL;

UPDATE templates
SET exit_node_id = template_exit_nodes.exit_node_id
FROM template_exit_nodes
WHERE template_exit_nodes.template_id = templates.id
	AND template_exit_nodes.position = 0;

COMMENT ON COLUMN templates.exit_node_id IS 'Exit node that terminates egress for workspaces built from this template. NULL routes egress directly.';

DROP TABLE template_exit_nodes;

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
