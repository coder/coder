BEGIN;

ALTER TABLE
	tailnet_clients
ADD COLUMN
	agent_ids uuid[];

UPDATE
	tailnet_clients
SET
	agent_ids = ARRAY[agent_id]::uuid[];

ALTER TABLE
	tailnet_clients
ALTER COLUMN
	agent_ids SET NOT NULL;


CREATE INDEX idx_tailnet_clients_agent_ids ON tailnet_clients USING GIN (agent_ids);

CREATE OR REPLACE FUNCTION tailnet_notify_client_change() RETURNS trigger
	LANGUAGE plpgsql
	AS $$
BEGIN
	-- check new first to get the updated agent ids.
	IF (NEW IS NOT NULL) THEN
		PERFORM pg_notify('tailnet_client_update', NEW.id || ',' || array_to_string(NEW.agent_ids, ','));
		RETURN NULL;
	END IF;
	IF (OLD IS NOT NULL) THEN
		PERFORM pg_notify('tailnet_client_update', OLD.id || ',' || array_to_string(OLD.agent_ids, ','));
		RETURN NULL;
	END IF;
END;
$$;

ALTER TABLE
	tailnet_clients
DROP COLUMN
	agent_id;

COMMIT;
