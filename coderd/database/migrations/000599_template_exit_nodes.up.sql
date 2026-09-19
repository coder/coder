CREATE TABLE template_exit_nodes (
	template_id uuid NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
	exit_node_id uuid NOT NULL REFERENCES exit_nodes(id) ON DELETE CASCADE,
	position int NOT NULL,
	PRIMARY KEY (template_id, exit_node_id),
	UNIQUE (template_id, position)
);

COMMENT ON TABLE template_exit_nodes IS 'Ordered exit nodes that terminate egress for workspaces built from a template.';
COMMENT ON COLUMN template_exit_nodes.position IS 'Zero-based preference order for exit node failover.';

INSERT INTO template_exit_nodes (template_id, exit_node_id, position)
SELECT id, exit_node_id, 0
FROM templates
WHERE exit_node_id IS NOT NULL;

DROP VIEW template_with_names;

ALTER TABLE templates DROP COLUMN exit_node_id;

CREATE VIEW template_with_names AS
SELECT templates.*,
	   ARRAY(
		   SELECT template_exit_nodes.exit_node_id
		   FROM template_exit_nodes
		   WHERE template_exit_nodes.template_id = templates.id
		   ORDER BY template_exit_nodes.position
	   )::uuid[] AS exit_node_ids,
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
