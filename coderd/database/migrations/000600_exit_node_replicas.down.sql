ALTER TABLE exit_nodes
	ADD COLUMN version text NOT NULL DEFAULT '',
	ADD COLUMN last_seen_at timestamptz,
	ADD COLUMN wireguard_endpoints text[] NOT NULL DEFAULT '{}';

COMMENT ON COLUMN exit_nodes.wireguard_endpoints IS 'Public ip:port pairs agents may use for direct WireGuard connections to the exit node.';

DROP TABLE exit_node_replicas;
