ALTER TABLE workspace_agents DROP COLUMN wireguard_node_ipv6;
ALTER TABLE workspace_agents DROP COLUMN wireguard_node_public_key;
ALTER TABLE workspace_agents DROP COLUMN wireguard_disco_public_key;
ALTER TABLE workspace_agents ADD COLUMN ip_addresses inet[] NOT NULL DEFAULT array[]::inet[];
ALTER TABLE workspace_agents ADD COLUMN node_public_key varchar(128);
ALTER TABLE workspace_agents ADD COLUMN disco_public_key varchar(128);
ALTER TABLE workspace_agents ADD COLUMN preferred_derp integer NOT NULL DEFAULT 0;
ALTER TABLE workspace_agents ADD COLUMN derp_latency jsonb NOT NULL DEFAULT '{}';
