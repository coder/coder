CREATE TABLE chat_project_instructions (
    project_id uuid PRIMARY KEY REFERENCES chat_projects(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    instructions text NOT NULL,
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chat_project_instructions_not_blank CHECK (length(btrim(instructions)) > 0)
);

COMMENT ON TABLE chat_project_instructions IS 'Per-project instructions injected into the system prompt of every chat in the project, for every user. A project without a row has no instructions.';
