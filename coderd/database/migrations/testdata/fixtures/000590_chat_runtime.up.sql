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
	'codex'::chat_runtime,
	t.id,
	true,
	'gpt-5.1-codex',
	'agent-full-access',
	NOW(),
	NOW()
FROM
	templates t
ORDER BY
	t.created_at, t.id
LIMIT 1
ON CONFLICT DO NOTHING;
