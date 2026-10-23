ALTER TABLE skills
    DROP CONSTRAINT skills_action_project_only,
    DROP COLUMN model_invocable,
    DROP COLUMN action;
