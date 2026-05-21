BEGIN TRANSACTION;
SELECT pg_advisory_xact_lock(7283699);

CREATE TABLE IF NOT EXISTS test_databases (
	name text PRIMARY KEY,
	created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
	dropped_at timestamp with time zone, -- null means it hasn't been dropped
	process_uuid uuid NOT NULL
);

CREATE INDEX IF NOT EXISTS test_databases_process_uuid ON test_databases (process_uuid, dropped_at);

ALTER TABLE test_databases ADD COLUMN IF NOT EXISTS test_name text;
COMMENT ON COLUMN test_databases.test_name IS 'Name of the test that created the database';
ALTER TABLE test_databases ADD COLUMN IF NOT EXISTS test_package text;
COMMENT ON COLUMN test_databases.test_package IS 'Package of the test that created the database';

COMMIT;
