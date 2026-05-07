ALTER TABLE chats
    ADD COLUMN workspace_auto_created BOOLEAN NOT NULL DEFAULT FALSE;

-- Backfill: mark chats whose workspace was created at or after the
-- chat itself. This mirrors the runtime heuristic that existed before
-- this column was introduced. The heuristic can only err in the safe
-- direction (showing a confirmation dialog that could be skipped),
-- never in the dangerous direction (skipping a dialog that should
-- appear), so the backfill preserves the existing user experience
-- for all previously created chats.
UPDATE chats
SET workspace_auto_created = TRUE
FROM workspaces
WHERE chats.workspace_id = workspaces.id
  AND workspaces.created_at >= chats.created_at;
