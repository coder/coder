-- Soft-delete stale `workspace_agents` rows.
--
-- Before v2.33.0, the auth path `GetWorkspaceAgentByInstanceID :one` silently
-- picked the newest matching row, so stale rows from earlier builds were
-- harmless. After #24325 replaced that with a `:many` lookup that rejects
-- ambiguity with HTTP 409, the accumulation becomes a hard failure: any
-- workspace whose EC2 instance hosted more than one build can no longer
-- re-authenticate its agent.
--
-- This migration backfills the "at most one non-deleted agent per workspace
-- that is itself not deleted" invariant over existing data. Going forward:
--   - `wsbuilder.Builder.Build` maintains it per-build via
--     `SoftDeletePriorWorkspaceAgents`.
--   - `provisionerdserver.CompleteJob` and `wsbuilder` also call
--     `SoftDeleteWorkspaceAgentsByWorkspaceID` when a workspace itself is
--     soft-deleted, so the table doesn't retain orphaned-but-non-deleted
--     agents referencing a deleted workspace.
--
-- Backfill scope:
--   1. Every agent belonging to a soft-deleted workspace -> deleted = TRUE.
--   2. For each still-live workspace, keep only agents belonging to the
--      current (highest build_number) build; soft-delete earlier builds'
--      agents.
--
-- Related:
--   #24325 (feature that regressed the behavior)
--   #24973 (partial fix, pool starvation)
--   #25031 (partial fix, handler cleanup + deleted-workspace filter)
--   #25155 (bug report)

-- 1. Soft-delete all agents on workspaces that are themselves deleted.
UPDATE workspace_agents
SET deleted = TRUE
WHERE id IN (
    SELECT wa.id
    FROM workspace_agents wa
    JOIN workspace_resources wr ON wr.id = wa.resource_id
    JOIN workspace_builds wb ON wb.job_id = wr.job_id
    JOIN workspaces w ON w.id = wb.workspace_id
    WHERE wa.deleted = FALSE
      AND w.deleted = TRUE
);

-- 2. For every live workspace, soft-delete agents not tied to the latest build.
WITH latest_builds AS (
    SELECT DISTINCT ON (workspace_id) id, workspace_id
    FROM workspace_builds
    ORDER BY workspace_id, build_number DESC
)
UPDATE workspace_agents
SET deleted = TRUE
WHERE id IN (
    SELECT wa.id
    FROM workspace_agents wa
    JOIN workspace_resources wr ON wr.id = wa.resource_id
    JOIN workspace_builds wb ON wb.job_id = wr.job_id
    JOIN workspaces w ON w.id = wb.workspace_id
    LEFT JOIN latest_builds lb ON lb.workspace_id = wb.workspace_id
    WHERE wa.deleted = FALSE
      AND w.deleted = FALSE
      AND (lb.id IS NULL OR wb.id <> lb.id)
);
