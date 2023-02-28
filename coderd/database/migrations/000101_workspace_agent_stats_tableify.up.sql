ALTER TABLE agent_stats	RENAME TO workspace_agent_stats;

ALTER TABLE	workspace_agent_stats ADD COLUMN connections_by_proto jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE	workspace_agent_stats ADD COLUMN connection_count integer DEFAULT 0 NOT NULL;
ALTER TABLE	workspace_agent_stats ADD COLUMN rx_packets integer DEFAULT 0 NOT NULL;
ALTER TABLE workspace_agent_stats ADD COLUMN rx_bytes integer DEFAULT 0 NOT NULL;
ALTER TABLE workspace_agent_stats ADD COLUMN tx_packets integer DEFAULT 0 NOT NULL;
ALTER TABLE workspace_agent_stats ADD COLUMN tx_bytes integer DEFAULT 0 NOT NULL;

UPDATE workspace_agent_stats SET
	connections_by_proto = coalesce((payload ->> 'conns_by_proto')::jsonb, '{}'::jsonb),
	connection_count = coalesce((payload ->> 'num_conns')::integer, 0),
	rx_packets = coalesce((payload ->> 'rx_packets')::integer, 0),
	rx_bytes = coalesce((payload ->> 'rx_bytes')::integer, 0),
	tx_packets = coalesce((payload ->> 'tx_packets')::integer, 0),
	tx_bytes = coalesce((payload ->> 'tx_bytes')::integer, 0);

ALTER TABLE workspace_agent_stats DROP COLUMN payload;
