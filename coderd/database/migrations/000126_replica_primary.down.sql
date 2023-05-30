BEGIN;

-- drop any rows that aren't primary replicas
DELETE FROM replicas
	WHERE "primary" = false;

ALTER TABLE replicas
	DROP COLUMN "primary";

COMMIT;
