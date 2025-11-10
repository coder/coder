DROP VIEW IF EXISTS tasks_with_status;
DROP TYPE IF EXISTS task_status;

DROP INDEX IF EXISTS tasks_organization_id_idx;
DROP INDEX IF EXISTS tasks_owner_id_idx;
DROP INDEX IF EXISTS tasks_workspace_id_idx;

ALTER TABLE task_workspace_apps
	DROP CONSTRAINT IF EXISTS task_workspace_apps_pkey;

-- Add back workspace_build_id column.
ALTER TABLE task_workspace_apps
	ADD COLUMN workspace_build_id UUID;

-- Try to populate workspace_build_id from workspace_builds.
UPDATE task_workspace_apps
SET workspace_build_id = workspace_builds.id
FROM workspace_builds
WHERE workspace_builds.build_number = task_workspace_apps.workspace_build_number
	AND workspace_builds.workspace_id IN (
		SELECT workspace_id FROM tasks WHERE tasks.id = task_workspace_apps.task_id
	);

-- Remove rows that couldn't be restored.
DELETE FROM task_workspace_apps
WHERE workspace_build_id IS NULL;

-- Restore original schema.
ALTER TABLE task_workspace_apps
	DROP COLUMN workspace_build_number,
	ALTER COLUMN workspace_build_id SET NOT NULL,
	ALTER COLUMN workspace_agent_id SET NOT NULL,
	ALTER COLUMN workspace_app_id SET NOT NULL;
