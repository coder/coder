-- Chains onto the identity fixture from 000565 and the earliest existing
-- workspace and workspace agents, since every column here is a foreign key.
INSERT INTO ai_sandboxes (
	id,
	workspace_id,
	parent_agent_id,
	child_agent_id,
	ai_agent_id,
	name,
	egress_enforcement,
	created_at,
	deleted
)
SELECT
	'57000000-0000-0000-0000-000000000001',
	workspaces.id,
	agents.id,
	agents.id,
	'56500000-0000-0000-0000-000000000002',
	'fixture-sandbox',
	'forced',
	'2026-01-01 00:00:00+00',
	false
FROM workspaces
CROSS JOIN (
	SELECT id FROM workspace_agents ORDER BY created_at, id LIMIT 1
) AS agents
ORDER BY workspaces.created_at, workspaces.id
LIMIT 1;
