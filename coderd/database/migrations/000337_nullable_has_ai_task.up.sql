-- The fields must be nullable because there's a period of time between
-- inserting a row into the database and finishing the "plan" provisioner job
-- when the final value of the field is unknown.
ALTER TABLE template_versions ALTER COLUMN has_ai_task DROP DEFAULT;
ALTER TABLE template_versions ALTER COLUMN has_ai_task DROP NOT NULL;
ALTER TABLE workspace_builds ALTER COLUMN has_ai_task DROP DEFAULT;
ALTER TABLE workspace_builds ALTER COLUMN has_ai_task DROP NOT NULL;
