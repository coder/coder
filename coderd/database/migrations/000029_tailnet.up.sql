ALTER TABLE workspace_agents DROP COLUMN wireguard_node_ipv6;
ALTER TABLE workspace_agents ADD COLUMN ip_addresses inet[] NOT NULL DEFAULT array[]::inet[];
ALTER TABLE workspace_agents RENAME COLUMN wireguard_node_public_key TO node_public_key;
ALTER TABLE workspace_agents RENAME COLUMN wireguard_disco_public_key TO disco_public_key;
ALTER TABLE workspace_agents ADD COLUMN derp varchar(128) NOT NULL DEFAULT '127.3.3.40:0';
ALTER TABLE workspace_agents ADD COLUMN derp_latency jsonb NOT NULL DEFAULT '{}';
