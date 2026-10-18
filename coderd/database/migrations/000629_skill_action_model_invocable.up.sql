-- action marks a project skill as a one-click action above project
-- composers. model_invocable controls whether agents see and load the skill
-- on their own; users can still invoke manual-only skills explicitly.
ALTER TABLE skills
    ADD COLUMN action boolean NOT NULL DEFAULT false,
    ADD COLUMN model_invocable boolean NOT NULL DEFAULT true,
    ADD CONSTRAINT skills_action_project_only CHECK (NOT action OR project_id IS NOT NULL);
