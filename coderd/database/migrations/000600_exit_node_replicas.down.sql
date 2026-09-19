ALTER TABLE exit_nodes
	ADD COLUMN version text NOT NULL DEFAULT '',
	ADD COLUMN last_seen_at timestamptz,
	ADD COLUMN wireguard_endpoints text[] NOT NULL DEFAULT '{}';

COMMENT ON COLUMN exit_nodes.wireguard_endpoints IS 'Public ip:port pairs agents may use for direct WireGuard connections to the exit node.';

UPDATE exit_nodes AS en
SET
	version = latest.version,
	last_seen_at = latest.updated_at,
	wireguard_endpoints = latest.wireguard_endpoints
FROM (
	SELECT DISTINCT ON (exit_node_id)
		exit_node_id, version, updated_at, wireguard_endpoints
	FROM exit_node_replicas
	ORDER BY exit_node_id, updated_at DESC, id DESC
) AS latest
WHERE latest.exit_node_id = en.id;

DROP TABLE exit_node_replicas;
