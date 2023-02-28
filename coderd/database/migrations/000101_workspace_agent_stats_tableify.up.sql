ALTER TABLE agent_stats
	RENAME TO workspace_agent_stats;

ALTER TABLE	workspace_agent_stats ADD COLUMN connections_by_proto jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE	workspace_agent_stats ADD COLUMN connection_count integer DEFAULT 0 NOT NULL;
ALTER TABLE	workspace_agent_stats ADD COLUMN rx_packets integer DEFAULT 0 NOT NULL;
ALTER TABLE workspace_agent_stats ADD COLUMN rx_bytes integer DEFAULT 0 NOT NULL;
ALTER TABLE workspace_agent_stats ADD COLUMN tx_packets integer DEFAULT 0 NOT NULL;
ALTER TABLE workspace_agent_stats ADD COLUMN tx_bytes integer DEFAULT 0 NOT NULL;

UPDATE workspace_agent_stats SET
	connections_by_proto = (payload ->> 'conns_by_proto')::jsonb,
	connection_count = (payload ->> 'num_conns')::integer,
	rx_packets = (payload ->> 'rx_packets')::integer,
	rx_bytes = (payload ->> 'rx_bytes')::integer,
	tx_packets = (payload ->> 'tx_packets')::integer,
	tx_bytes = (payload ->> 'tx_bytes')::integer;

ALTER TABLE workspace_agent_stats DROP COLUMN payload;
