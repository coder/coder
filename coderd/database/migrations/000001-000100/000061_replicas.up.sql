CREATE TABLE IF NOT EXISTS replicas (
    -- A unique identifier for the replica that is stored on disk.
    -- For persistent replicas, this will be reused.
    -- For ephemeral replicas, this will be a new UUID for each one.
    id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    -- The time the replica was created.
    started_at timestamp with time zone NOT NULL,
    -- The time the replica was last seen.
    stopped_at timestamp with time zone,
    -- Updated periodically to ensure the replica is still alive.
    updated_at timestamp with time zone NOT NULL,
    -- Hostname is the hostname of the replica.
    hostname text NOT NULL,
    -- Region is the region the replica is in.
    -- We only DERP mesh to the same region ID of a running replica.
    region_id integer NOT NULL,
    -- An address that should be accessible to other replicas.
    relay_address text NOT NULL,
    -- The latency of the replica to the database in microseconds.
    database_latency int NOT NULL,
    -- Version is the Coder version of the replica.
    version text NOT NULL,
    error text NOT NULL DEFAULT ''
);

-- Associates a provisioner daemon with a replica.
ALTER TABLE provisioner_daemons ADD COLUMN replica_id uuid;
