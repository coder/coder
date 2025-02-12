ALTER TABLE workspace_agent_volume_resource_monitors
	DROP COLUMN updated_at,
	DROP COLUMN state,
	DROP COLUMN debounced_until;

ALTER TABLE workspace_agent_memory_resource_monitors
	DROP COLUMN updated_at,
	DROP COLUMN state,
	DROP COLUMN debounced_until;

DROP TYPE workspace_agent_monitor_state;
