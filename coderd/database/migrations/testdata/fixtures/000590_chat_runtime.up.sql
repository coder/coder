INSERT INTO chat_runtime_configs (
	organization_id,
	runtime,
	template_id,
	enabled,
	model,
	permission_mode,
	created_at,
	updated_at
)
SELECT
	t.organization_id,
	'claude_code'::chat_runtime,
	t.id,
	true,
	'claude-sonnet-4-5',
	'acceptEdits',
	NOW(),
	NOW()
FROM
	templates t
ORDER BY
	t.created_at, t.id
LIMIT 1
ON CONFLICT DO NOTHING;
