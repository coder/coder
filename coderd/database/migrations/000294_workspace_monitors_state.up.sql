CREATE TYPE workspace_agent_monitor_state AS ENUM (
	'OK',
	'NOK'
);

ALTER TABLE workspace_agent_memory_resource_monitors
	ADD COLUMN updated_at      timestamp with time zone      NOT NULL DEFAULT CURRENT_TIMESTAMP,
	ADD COLUMN state           workspace_agent_monitor_state NOT NULL DEFAULT 'OK',
	ADD COLUMN debounced_until timestamp with time zone      NOT NULL DEFAULT '0001-01-01 00:00:00'::timestamptz;

ALTER TABLE workspace_agent_volume_resource_monitors
	ADD COLUMN updated_at      timestamp with time zone      NOT NULL DEFAULT CURRENT_TIMESTAMP,
	ADD COLUMN state           workspace_agent_monitor_state NOT NULL DEFAULT 'OK',
	ADD COLUMN debounced_until timestamp with time zone      NOT NULL DEFAULT '0001-01-01 00:00:00'::timestamptz;
