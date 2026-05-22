DROP VIEW IF EXISTS chats_expanded;

ALTER TABLE chats
    ADD COLUMN user_acl jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN group_acl jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE chats
    ADD CONSTRAINT chat_user_acl_not_null_jsonb
        CHECK (user_acl IS NOT NULL AND jsonb_typeof(user_acl) = 'object'),
    ADD CONSTRAINT chat_group_acl_not_null_jsonb
        CHECK (group_acl IS NOT NULL AND jsonb_typeof(group_acl) = 'object'),
    ADD CONSTRAINT chat_acl_only_on_root_chats
        CHECK (
            (parent_chat_id IS NULL AND root_chat_id IS NULL)
            OR (
                user_acl = '{}'::jsonb
                AND group_acl = '{}'::jsonb
            )
        );

CREATE VIEW chats_expanded AS
SELECT
    c.id,
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
    c.archived,
    c.last_error,
    c.mode,
    c.mcp_server_ids,
    c.labels,
    c.build_id,
    c.agent_id,
    c.pin_order,
    c.last_read_message_id,
    c.last_injected_context,
    c.dynamic_tools,
    c.organization_id,
    c.plan_mode,
    c.client_type,
    c.last_turn_summary,
    COALESCE(root.user_acl, c.user_acl) AS user_acl,
    COALESCE(root.group_acl, c.group_acl) AS group_acl,
    owner.username AS owner_username,
    owner.name AS owner_name
FROM
    chats c
    LEFT JOIN chats root ON root.id = COALESCE(c.root_chat_id, c.parent_chat_id)
    JOIN visible_users owner ON owner.id = c.owner_id;

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat:share';
