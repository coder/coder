-- Add owner_id to boundary_sessions to avoid expensive JOINs when
-- deriving the workspace owner for RBAC checks during log insertion.
ALTER TABLE boundary_sessions ADD COLUMN owner_id uuid;

-- Backfill owner_id from the workspace agent -> workspace -> owner chain.
UPDATE boundary_sessions bs
SET owner_id = w.owner_id
FROM workspace_agents wa
JOIN workspace_resources wr ON wa.resource_id = wr.id
JOIN provisioner_jobs pj ON wr.job_id = pj.id
JOIN workspace_builds wb ON pj.id = wb.job_id
JOIN workspaces w ON wb.workspace_id = w.id
WHERE wa.id = bs.workspace_agent_id
  AND wa.deleted = FALSE
  AND pj.type = 'workspace_build'
  AND w.deleted = FALSE;

-- Delete any sessions that could not be backfilled (orphaned data).
DELETE FROM boundary_sessions WHERE owner_id IS NULL;

-- Add FK constraint. SET NULL preserves audit data when a user is
-- hard-deleted; the session and its logs survive with a NULL owner.
ALTER TABLE boundary_sessions
    ADD CONSTRAINT boundary_sessions_owner_id_fkey
    FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE SET NULL;
