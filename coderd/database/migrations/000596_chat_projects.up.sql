ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'chat_project';

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_project:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_project:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_project:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_project:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_project:delete';

CREATE TABLE chat_projects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chat_projects_name_not_blank CHECK (length(trim(name)) > 0)
);

COMMENT ON TABLE chat_projects IS 'Organization-scoped projects that group agent chats.';

CREATE INDEX idx_chat_projects_organization_id ON chat_projects (organization_id);
CREATE UNIQUE INDEX idx_chat_projects_org_lower_name ON chat_projects (organization_id, lower(name));

ALTER TABLE chats ADD COLUMN project_id uuid REFERENCES chat_projects(id) ON DELETE SET NULL;
COMMENT ON COLUMN chats.project_id IS 'Optional project that groups a root chat with related chats.';
CREATE INDEX idx_chats_project_id ON chats (project_id) WHERE project_id IS NOT NULL;

-- Recreate chats_expanded: its explicit column list hides new columns otherwise.
DROP VIEW IF EXISTS chats_expanded;

CREATE VIEW chats_expanded AS
 SELECT c.id,
    c.owner_id,
    c.workspace_id,
    c.title,
    c.status,
    c.worker_id,
    c.started_at,
    c.heartbeat_at,
    c.created_at,
    c.updated_at,
    c.parent_chat_id,
    c.root_chat_id,
    c.last_model_config_id,
    c.last_reasoning_effort,
    c.archived,
    c.last_error,
    c.mode,
    c.mcp_server_ids,
    c.labels,
    c.build_id,
    c.agent_id,
    c.pin_order,
    c.last_read_message_id,
    c.dynamic_tools,
    c.organization_id,
    c.project_id,
    c.plan_mode,
    c.client_type,
    c.last_turn_summary,
    c.summary,
    c.summary_generated_at,
    c.snapshot_version,
    c.history_version,
    c.queue_version,
    c.generation_attempt,
    c.retry_state,
    c.retry_state_version,
    c.runner_id,
    c.requires_action_deadline_at,
    COALESCE(root.user_acl, c.user_acl) AS user_acl,
    COALESCE(root.group_acl, c.group_acl) AS group_acl,
    owner.username AS owner_username,
    owner.name AS owner_name,
    c.context_aggregate_hash,
    c.context_dirty_since,
    c.context_dirty_resources,
    c.context_error,
    c.compaction_requested_at
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));
