COMMENT ON COLUMN replicas.relay_address IS 'URL for DERP relays.';
ALTER TABLE replicas ADD COLUMN cluster_host text DEFAULT ''::text NOT NULL;
COMMENT ON COLUMN replicas.cluster_host IS 'Hostname or IP address the replica is reachable at for clustering purposes.';
ALTER TABLE replicas ADD COLUMN nats_port integer DEFAULT 0 NOT NULL CONSTRAINT nats_port_valid_tcp CHECK ( nats_port >= 0 AND nats_port <= 65535);
COMMENT ON COLUMN replicas.nats_port IS 'Port number for NATS clustering. 0 means NATS is disabled.';
