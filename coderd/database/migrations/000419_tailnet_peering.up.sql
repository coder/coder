CREATE TABLE agent_peering_ids (
    agent_id uuid NOT NULL,
    peering_id bytea NOT NULL,
    PRIMARY KEY (agent_id, peering_id)
);

CREATE TABLE tailnet_peering_events (
    peering_id bytea NOT NULL,
    event_type text NOT NULL,
    src_peer_id uuid,
    dst_peer_id uuid,
    node bytea,
    occurred_at timestamp with time zone NOT NULL
);
