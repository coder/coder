BEGIN;

ALTER TABLE
	tailnet_clients
ADD COLUMN
	agent_id uuid;

UPDATE
	tailnet_clients
SET
	-- there's no reason for us to try and preserve data since coordinators will
	-- have to restart anyways, which will create all of the client mappings.
	agent_id = '00000000-0000-0000-0000-000000000000'::uuid;

ALTER TABLE
	tailnet_clients
ALTER COLUMN
	agent_id SET NOT NULL;

DROP TABLE tailnet_client_subscriptions;
DROP FUNCTION tailnet_notify_client_subscription_change;

-- update the tailnet_clients trigger to the old version.
CREATE OR REPLACE FUNCTION tailnet_notify_client_change() RETURNS trigger
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

COMMIT;
