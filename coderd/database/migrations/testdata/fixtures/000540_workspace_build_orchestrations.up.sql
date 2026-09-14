INSERT INTO workspace_build_orchestrations (
	id,
	created_at,
	updated_at,
	workspace_id,
	parent_build_id,
	child_transition
)
SELECT
	'4e983a68-9b8a-4d4e-a4d6-5f2dd73551c2'::uuid,
	NOW(),
	NOW(),
	workspace_id,
	id,
	'start'::workspace_transition
FROM
	workspace_builds
ORDER BY
	created_at, id
LIMIT 1
ON CONFLICT DO NOTHING;
