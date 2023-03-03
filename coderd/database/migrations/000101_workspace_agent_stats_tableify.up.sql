ALTER TABLE agent_stats	RENAME TO workspace_agent_stats;

ALTER TABLE	workspace_agent_stats ADD COLUMN connections_by_proto jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE	workspace_agent_stats ADD COLUMN connection_count bigint DEFAULT 0 NOT NULL;
ALTER TABLE	workspace_agent_stats ADD COLUMN rx_packets bigint DEFAULT 0 NOT NULL;
ALTER TABLE workspace_agent_stats ADD COLUMN rx_bytes bigint DEFAULT 0 NOT NULL;
ALTER TABLE workspace_agent_stats ADD COLUMN tx_packets bigint DEFAULT 0 NOT NULL;
ALTER TABLE workspace_agent_stats ADD COLUMN tx_bytes bigint DEFAULT 0 NOT NULL;

UPDATE workspace_agent_stats SET
	connections_by_proto = coalesce((payload ->> 'conns_by_proto')::jsonb, '{}'::jsonb),
	connection_count = coalesce((payload ->> 'num_conns')::bigint, 0),
	rx_packets = coalesce((payload ->> 'rx_packets')::bigint, 0),
	rx_bytes = coalesce((payload ->> 'rx_bytes')::bigint, 0),
	tx_packets = coalesce((payload ->> 'tx_packets')::bigint, 0),
	tx_bytes = coalesce((payload ->> 'tx_bytes')::bigint, 0);

ALTER TABLE workspace_agent_stats DROP COLUMN payload;
