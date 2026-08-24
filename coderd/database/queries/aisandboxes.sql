-- name: InsertAISandbox :one
INSERT INTO ai_sandboxes (
	id,
	workspace_id,
	parent_agent_id,
	child_agent_id,
	ai_agent_id,
	name,
	egress_enforcement,
	created_at,
	occupancy_count
) VALUES (
	@id,
	@workspace_id,
	@parent_agent_id,
	@child_agent_id,
	@ai_agent_id,
	@name,
	@egress_enforcement,
	@created_at,
	1
) RETURNING *;

-- name: GetAISandboxByParentAgentAndName :one
SELECT * FROM ai_sandboxes
WHERE parent_agent_id = @parent_agent_id
  AND name = @name
  AND NOT deleted;

-- name: GetAISandboxByID :one
SELECT * FROM ai_sandboxes WHERE id = @id;

-- name: GetAISandboxesByParentAgentID :many
SELECT * FROM ai_sandboxes
WHERE parent_agent_id = @parent_agent_id
  AND NOT deleted
ORDER BY created_at ASC;

-- name: GetAISandboxesByWorkspaceID :many
SELECT * FROM ai_sandboxes
WHERE workspace_id = @workspace_id
  AND NOT deleted
ORDER BY created_at ASC;

-- name: SoftDeleteAISandbox :exec
-- Marks a sandbox destroyed and vacates it. The child agent row is
-- soft-deleted and its keys revoked separately so the record survives for
-- correlation.
--
-- Destroyed and empty are two facts, and they coincide only because nothing
-- empties a sandbox without destroying it. Writing both says which is which
-- rather than leaving `deleted` to stand for two things.
UPDATE ai_sandboxes SET deleted = true, occupancy_count = 0 WHERE id = @id;
