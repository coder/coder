BEGIN;

ALTER TABLE
	tailnet_clients
ADD COLUMN
	agent_id uuid;

UPDATE
	tailnet_clients
SET
	-- grab just the first agent_id, or default to an empty UUID.
	agent_id = COALESCE(agent_ids[0], '00000000-0000-0000-0000-000000000000'::uuid);

ALTER TABLE
	tailnet_clients
ALTER COLUMN
	agent_id SET NOT NULL;

ALTER TABLE
	tailnet_clients
DROP COLUMN
	agent_ids;

COMMIT;
