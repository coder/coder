-- WARNING: Restoring this constraint after running a newer version of coderd
-- and using tasks is bound to break this constraint.
ALTER TABLE workspace_builds
ADD CONSTRAINT workspace_builds_ai_task_sidebar_app_id_required CHECK (((((has_ai_task IS NULL) OR (has_ai_task = false)) AND (ai_task_sidebar_app_id IS NULL)) OR ((has_ai_task = true) AND (ai_task_sidebar_app_id IS NOT NULL))));
