-- Explain two existing build deadlines so later migrations and rollbacks run
-- against stored activity sources and an actor reference.
UPDATE workspace_builds
SET deadline_reason = 'activity', deadline_changed_at = NOW(),
	deadline_sources = '[{"type": "connection", "agent_id": "5e2d1b9c-6a1f-4c7e-9b3a-2f8d4c6e1a70"}, {"type": "chat"}]'::jsonb
WHERE id = (SELECT id FROM workspace_builds ORDER BY created_at, id LIMIT 1);

UPDATE workspace_builds
SET deadline_reason = 'user_changed', deadline_changed_at = NOW(),
	deadline_changed_by = (SELECT id FROM users ORDER BY created_at, id LIMIT 1)
WHERE id = (SELECT id FROM workspace_builds ORDER BY created_at, id LIMIT 1 OFFSET 1);
