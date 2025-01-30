INSERT INTO workspace_monitors (
	workspace_id,
	monitor_type,
	state,
	created_at,
	updated_at,
	debounced_until
) VALUES (
	(SELECT id FROM workspaces WHERE deleted = FALSE LIMIT 1),
	'memory',
	'OK',
	NOW(),
	NOW(),
	NOW()
);
