-- Migration 485 retypes chats.last_error to jsonb and backfills legacy
-- text rows into the structured persisted payload shape.
DO $$
DECLARE
	payload jsonb;
BEGIN
	SELECT last_error INTO STRICT payload
	FROM chats
	WHERE id = '72c0438a-18eb-4688-ab80-e4c6a126ef96';

	IF payload ->> 'message' <> 'Legacy provider failure' THEN
		RAISE EXCEPTION 'expected migrated last_error message, got %',
			payload ->> 'message';
	END IF;

	IF payload ->> 'kind' <> 'generic' THEN
		RAISE EXCEPTION 'expected migrated last_error kind, got %',
			payload ->> 'kind';
	END IF;

	PERFORM 1
	FROM chats
	WHERE id = '5a4ac6a3-9dc5-440f-ae6b-5805e477bc59'
		AND last_error IS NULL;
	IF NOT FOUND THEN
		RAISE EXCEPTION 'expected null last_error row to remain NULL after migration';
	END IF;
END $$;
