CREATE TABLE tailnet_coordinators (
	id uuid NOT NULL PRIMARY KEY,
	heartbeat_at timestamp with time zone NOT NULL
);

COMMENT ON TABLE tailnet_coordinators IS 'We keep this separate from replicas in case we need to break the coordinator out into its own service';

CREATE TABLE tailnet_clients (
	id uuid NOT NULL,
	coordinator_id uuid NOT NULL,
	agent_id uuid NOT NULL,
	updated_at timestamp with time zone NOT NULL,
	node jsonb NOT NULL,
	PRIMARY KEY (id, coordinator_id),
    FOREIGN KEY (coordinator_id) REFERENCES tailnet_coordinators(id) ON DELETE CASCADE
);


-- For querying/deleting mappings
CREATE INDEX idx_tailnet_clients_agent ON tailnet_clients (agent_id);

-- For shutting down / GC a coordinator
CREATE INDEX idx_tailnet_clients_coordinator ON tailnet_clients (coordinator_id);

CREATE TABLE tailnet_agents (
	id uuid NOT NULL,
	coordinator_id uuid NOT NULL,
	updated_at timestamp with time zone NOT NULL,
	node jsonb NOT NULL,
    PRIMARY KEY (id, coordinator_id),
    FOREIGN KEY (coordinator_id) REFERENCES tailnet_coordinators(id) ON DELETE CASCADE
);


-- For shutting down / GC a coordinator
CREATE INDEX idx_tailnet_agents_coordinator ON tailnet_agents (coordinator_id);

-- Any time the tailnet_clients table changes, send an update with the affected client and agent IDs
CREATE FUNCTION tailnet_notify_client_change() RETURNS trigger
	LANGUAGE plpgsql
	AS $$
BEGIN
	IF (OLD IS NOT NULL) THEN
		PERFORM pg_notify('tailnet_client_update', OLD.id || ',' || OLD.agent_id);
		RETURN NULL;
	END IF;
	IF (NEW IS NOT NULL) THEN
		PERFORM pg_notify('tailnet_client_update', NEW.id || ',' || NEW.agent_id);
		RETURN NULL;
	END IF;
END;
$$;

CREATE TRIGGER tailnet_notify_client_change
	AFTER INSERT OR UPDATE OR DELETE ON tailnet_clients
	FOR EACH ROW
EXECUTE PROCEDURE tailnet_notify_client_change();

-- Any time tailnet_agents table changes, send an update with the affected agent ID.
CREATE FUNCTION tailnet_notify_agent_change() RETURNS trigger
	LANGUAGE plpgsql
	AS $$
BEGIN
	IF (OLD IS NOT NULL) THEN
		PERFORM pg_notify('tailnet_agent_update', OLD.id::text);
		RETURN NULL;
	END IF;
	IF (NEW IS NOT NULL) THEN
		PERFORM pg_notify('tailnet_agent_update', NEW.id::text);
		RETURN NULL;
	END IF;
END;
$$;

CREATE TRIGGER tailnet_notify_agent_change
	AFTER INSERT OR UPDATE OR DELETE ON tailnet_agents
	FOR EACH ROW
EXECUTE PROCEDURE tailnet_notify_agent_change();

-- Send coordinator heartbeats
CREATE FUNCTION tailnet_notify_coordinator_heartbeat() RETURNS trigger
	LANGUAGE plpgsql
AS $$
BEGIN
	PERFORM pg_notify('tailnet_coordinator_heartbeat', NEW.id::text);
	RETURN NULL;
END;
$$;

CREATE TRIGGER tailnet_notify_coordinator_heartbeat
	AFTER INSERT OR UPDATE ON tailnet_coordinators
	FOR EACH ROW
EXECUTE PROCEDURE tailnet_notify_coordinator_heartbeat();
