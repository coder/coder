-- It is not possible to delete a value from an enum, so we have to recreate it.
CREATE TYPE old_workspace_app_status_state AS ENUM ('working', 'complete', 'failure');

-- Convert the new "idle" state into "complete".  This means we lose some
-- information when downgrading, but this is necessary to swap to the old enum.
UPDATE workspace_app_statuses SET state = 'complete' WHERE state = 'idle';

-- Swap to the old enum.
ALTER TABLE workspace_app_statuses
ALTER COLUMN state TYPE old_workspace_app_status_state
USING (state::text::old_workspace_app_status_state);

-- Drop the new enum and rename the old one to the final name.
DROP TYPE workspace_app_status_state;
ALTER TYPE old_workspace_app_status_state RENAME TO workspace_app_status_state;
