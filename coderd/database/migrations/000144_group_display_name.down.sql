BEGIN;

ALTER TABLE groups
	DROP COLUMN display_name;

COMMIT;
