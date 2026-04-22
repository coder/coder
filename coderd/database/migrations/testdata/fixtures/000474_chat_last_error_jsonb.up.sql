-- Migration 474 retypes chats.last_error to jsonb and backfills legacy
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
END $$;
