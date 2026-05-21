ALTER TABLE workspace_agent_stats RENAME TO agent_stats;

ALTER TABLE agent_stats ADD COLUMN payload jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE agent_stats DROP COLUMN connections_by_proto,
	DROP COLUMN connection_count,
	DROP COLUMN rx_packets,
	DROP COLUMN rx_bytes,
	DROP COLUMN tx_packets,
	DROP COLUMN tx_bytes;
