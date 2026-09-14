-- We no longer need to enforce this constraint as tasks have their own data
-- model.
ALTER TABLE workspace_builds
DROP CONSTRAINT workspace_builds_ai_task_sidebar_app_id_required;
