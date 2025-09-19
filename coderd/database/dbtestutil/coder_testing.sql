CREATE TABLE IF NOT EXISTS test_databases (
	name text PRIMARY KEY,
	created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
	dropped_at timestamp with time zone, -- null means it hasn't been dropped
	process_uuid uuid NOT NULL,
	-- these are both unused for now, but we'd like to include them for statistics later
	test_package text,
	test_name text
);

CREATE INDEX IF NOT EXISTS test_databases_process_uuid ON test_databases (process_uuid, dropped_at);
