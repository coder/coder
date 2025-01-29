CREATE TYPE workspace_monitor_state AS ENUM (
	'OK',
	'NOK'
);

CREATE TYPE workspace_monitor_type AS ENUM (
	'memory',
	'volume'
);

CREATE TABLE workspace_monitors (
	workspace_id uuid NOT NULL,
	monitor_type workspace_monitor_type NOT NULL,
	volume_path text CHECK (monitor_type = 'volume'),
	state workspace_monitor_state NOT NULL,
	created_at timestamp with time zone NOT NULL,
	updated_at timestamp with time zone NOT NULL,
	debounced_until timestamp with time zone NOT NULL
);
