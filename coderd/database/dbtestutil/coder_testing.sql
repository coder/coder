CREATE TABLE IF NOT EXISTS test_databases (
	name text PRIMARY KEY,
	created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
	dropped_at timestamp with time zone, -- null means it hasn't been dropped
	process_uuid uuid NOT NULL
);

CREATE INDEX IF NOT EXISTS test_databases_process_uuid ON test_databases (process_uuid, dropped_at);
