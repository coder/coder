-- Restore tailnet v1 API tables (unused, but required for rollback).

-- Create tables.
CREATE TABLE tailnet_clients (
    id uuid NOT NULL,
    coordinator_id uuid NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    node jsonb NOT NULL,
    PRIMARY KEY (id, coordinator_id),
    FOREIGN KEY (coordinator_id) REFERENCES tailnet_coordinators (id) ON DELETE CASCADE
);

CREATE TABLE tailnet_agents (
    id uuid NOT NULL,
    coordinator_id uuid NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    node jsonb NOT NULL,
    PRIMARY KEY (id, coordinator_id),
    FOREIGN KEY (coordinator_id) REFERENCES tailnet_coordinators (id) ON DELETE CASCADE
);

CREATE TABLE tailnet_client_subscriptions (
    client_id uuid NOT NULL,
    coordinator_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    PRIMARY KEY (client_id, coordinator_id, agent_id),
    FOREIGN KEY (coordinator_id) REFERENCES tailnet_coordinators (id) ON DELETE CASCADE
);

-- Create indexes.
CREATE INDEX idx_tailnet_agents_coordinator ON tailnet_agents USING btree (coordinator_id);
CREATE INDEX idx_tailnet_clients_coordinator ON tailnet_clients USING btree (coordinator_id);

-- Create trigger functions.
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

CREATE FUNCTION tailnet_notify_client_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
	var_client_id uuid;
	var_coordinator_id uuid;
	var_agent_ids uuid[];
	var_agent_id uuid;
BEGIN
	IF (NEW.id IS NOT NULL) THEN
		var_client_id = NEW.id;
		var_coordinator_id = NEW.coordinator_id;
	ELSIF (OLD.id IS NOT NULL) THEN
		var_client_id = OLD.id;
		var_coordinator_id = OLD.coordinator_id;
	END IF;

	-- Read all agents the client is subscribed to, so we can notify them.
	SELECT
		array_agg(agent_id)
	INTO
		var_agent_ids
	FROM
		tailnet_client_subscriptions subs
	WHERE
		subs.client_id = NEW.id AND
		subs.coordinator_id = NEW.coordinator_id;

	-- No agents to notify
	if (var_agent_ids IS NULL) THEN
		return NULL;
	END IF;

	-- pg_notify is limited to 8k bytes, which is approximately 221 UUIDs.
	-- Instead of sending all agent ids in a single update, send one for each
	-- agent id to prevent overflow.
	FOREACH var_agent_id IN ARRAY var_agent_ids
	LOOP
		PERFORM pg_notify('tailnet_client_update', var_client_id || ',' || var_agent_id);
	END LOOP;

	return NULL;
END;
$$;

CREATE FUNCTION tailnet_notify_client_subscription_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	IF (NEW IS NOT NULL) THEN
		PERFORM pg_notify('tailnet_client_update', NEW.client_id || ',' || NEW.agent_id);
		RETURN NULL;
	ELSIF (OLD IS NOT NULL) THEN
		PERFORM pg_notify('tailnet_client_update', OLD.client_id || ',' || OLD.agent_id);
		RETURN NULL;
	END IF;
END;
$$;

-- Create triggers.
CREATE TRIGGER tailnet_notify_agent_change
    AFTER INSERT OR DELETE OR UPDATE ON tailnet_agents
    FOR EACH ROW
    EXECUTE FUNCTION tailnet_notify_agent_change();

CREATE TRIGGER tailnet_notify_client_change
    AFTER INSERT OR DELETE OR UPDATE ON tailnet_clients
    FOR EACH ROW
    EXECUTE FUNCTION tailnet_notify_client_change();

CREATE TRIGGER tailnet_notify_client_subscription_change
    AFTER INSERT OR DELETE OR UPDATE ON tailnet_client_subscriptions
    FOR EACH ROW
    EXECUTE FUNCTION tailnet_notify_client_subscription_change();
