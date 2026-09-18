CREATE TABLE exit_nodes (
	id uuid NOT NULL PRIMARY KEY,
	organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
	name text NOT NULL,
	display_name text NOT NULL DEFAULT '',
	created_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL,
	deleted boolean NOT NULL DEFAULT false,
	token_hashed_secret bytea NOT NULL,
	version text NOT NULL DEFAULT '',
	last_seen_at timestamptz,
	wireguard_endpoints text[] NOT NULL DEFAULT '{}'
);

COMMENT ON TABLE exit_nodes IS 'Tailnet peers that terminate workspace egress, enforce policy, and report flows.';
COMMENT ON COLUMN exit_nodes.deleted IS 'Boolean indicator of a deleted exit node. Exit nodes are soft-deleted.';
COMMENT ON COLUMN exit_nodes.token_hashed_secret IS 'Hashed secret used to authenticate the exit node to coderd.';
COMMENT ON COLUMN exit_nodes.wireguard_endpoints IS 'Public ip:port pairs agents may use for direct WireGuard connections to the exit node.';

CREATE UNIQUE INDEX exit_nodes_organization_id_lower_name_idx ON exit_nodes USING btree (organization_id, lower(name)) WHERE deleted = false;

ALTER TYPE connection_type ADD VALUE IF NOT EXISTS 'egress';

ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'exit_node';

ALTER TABLE templates
	ADD COLUMN exit_node_id uuid REFERENCES exit_nodes(id) ON DELETE SET NULL,
	ADD COLUMN exit_node_enforce boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN templates.exit_node_id IS 'Exit node that terminates egress for workspaces built from this template. NULL routes egress directly.';
COMMENT ON COLUMN templates.exit_node_enforce IS 'Whether agents transparently enforce that workspace egress goes through the exit node.';

DROP VIEW template_with_names;

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
