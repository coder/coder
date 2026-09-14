-- Add owner_id to boundary_sessions to avoid expensive JOINs when
-- deriving the workspace owner for RBAC checks during log insertion.
ALTER TABLE boundary_sessions ADD COLUMN owner_id uuid;

COMMENT ON COLUMN boundary_sessions.owner_id IS 'The ID of the user who owns the workspace. NULL if the user has been deleted.';

-- Backfill owner_id from the workspace agent -> workspace -> owner chain.
-- Soft-deleted agents and workspaces are included so that their audit
-- data is preserved.
UPDATE boundary_sessions bs
SET owner_id = w.owner_id
FROM workspace_agents wa
JOIN workspace_resources wr ON wa.resource_id = wr.id
JOIN provisioner_jobs pj ON wr.job_id = pj.id
JOIN workspace_builds wb ON pj.id = wb.job_id
JOIN workspaces w ON wb.workspace_id = w.id
WHERE wa.id = bs.workspace_agent_id
  AND pj.type = 'workspace_build';

-- Delete any sessions that could not be backfilled (orphaned data
-- with no resolvable workspace agent or workspace build chain).
DELETE FROM boundary_sessions WHERE owner_id IS NULL;

-- Add FK constraint. SET NULL preserves audit data when a user is
-- hard-deleted; the session and its logs survive with a NULL owner.
ALTER TABLE boundary_sessions
    ADD CONSTRAINT boundary_sessions_owner_id_fkey
    FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE SET NULL;
