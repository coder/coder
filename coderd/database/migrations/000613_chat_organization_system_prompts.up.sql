ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'chat_organization_system_prompt';

CREATE TABLE chat_organization_system_prompts (
    organization_id uuid PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    system_prompt text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE chat_organization_system_prompts IS 'Organization-scoped system prompts added after the deployment system prompt when Coder Agents chats are created.';
