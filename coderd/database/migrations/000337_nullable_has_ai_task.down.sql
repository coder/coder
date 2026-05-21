ALTER TABLE template_versions ALTER COLUMN has_ai_task SET DEFAULT false;
ALTER TABLE template_versions ALTER COLUMN has_ai_task SET NOT NULL;
ALTER TABLE workspace_builds ALTER COLUMN has_ai_task SET DEFAULT false;
ALTER TABLE workspace_builds ALTER COLUMN has_ai_task SET NOT NULL;
