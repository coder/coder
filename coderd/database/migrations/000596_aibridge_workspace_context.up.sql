-- Attach optional workspace context to an AI Bridge interception.
-- Two valid states:
--   (1) NULL     - workspace unknown (default, backwards-compatible)
--   (2) non-NULL - workspace ID recorded at interception time
--
-- No foreign key constraint as historical attribution must survive deletion of
-- the attributed workspace.

ALTER TABLE aibridge_interceptions
    ADD COLUMN workspace_id UUID NULL;

COMMENT ON COLUMN aibridge_interceptions.workspace_id IS
    'The workspace in which the agent ran. NULL when no workspace context is available.';
