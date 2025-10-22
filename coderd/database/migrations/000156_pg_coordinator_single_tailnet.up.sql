CREATE TABLE tailnet_client_subscriptions (
	client_id uuid NOT NULL,
	coordinator_id uuid NOT NULL,
	-- this isn't a foreign key since it's more of a list of agents the client
	-- *wants* to connect to, and they don't necessarily have to currently
	-- exist in the db.
	agent_id uuid NOT NULL,
	updated_at timestamp with time zone NOT NULL,
	PRIMARY KEY (client_id, coordinator_id, agent_id),
	FOREIGN KEY (coordinator_id)  REFERENCES tailnet_coordinators (id) ON DELETE CASCADE
	-- we don't keep a foreign key to the tailnet_clients table since there's
	-- not a great way to guarantee that a subscription is always added after
	-- the client is inserted. clients are only created after the client sends
	-- its first node update, which can take an undetermined amount of time.
);

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

CREATE TRIGGER tailnet_notify_client_subscription_change
	AFTER INSERT OR UPDATE OR DELETE ON tailnet_client_subscriptions
	FOR EACH ROW
EXECUTE PROCEDURE tailnet_notify_client_subscription_change();

CREATE OR REPLACE FUNCTION tailnet_notify_client_change() RETURNS trigger
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

ALTER TABLE
	tailnet_clients
DROP COLUMN
	agent_id;
