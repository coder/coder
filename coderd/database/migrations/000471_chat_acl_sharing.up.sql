-- Add user/group ACL columns to chats for read-only sharing with users and groups.
ALTER TABLE chats
    ADD COLUMN user_acl  jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN group_acl jsonb NOT NULL DEFAULT '{}'::jsonb;

-- Reject NULL jsonb objects so downstream views and Rego->SQL treat the column as a map.
ALTER TABLE chats
    ADD CONSTRAINT chat_user_acl_not_null_jsonb
        CHECK (user_acl IS NOT NULL AND jsonb_typeof(user_acl) = 'object'),
    ADD CONSTRAINT chat_group_acl_not_null_jsonb
        CHECK (group_acl IS NOT NULL AND jsonb_typeof(group_acl) = 'object');

-- chats_with_acl projects each chat alongside its effective ACL:
-- COALESCE to the root chat's ACL for sub-chats, falling back to the
-- chat's own ACL for roots (and for orphaned sub-chats).
CREATE VIEW chats_with_acl AS
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
    COALESCE(root.user_acl,  c.user_acl)  AS user_acl,
    COALESCE(root.group_acl, c.group_acl) AS group_acl
FROM
    chats c
    LEFT JOIN chats root ON root.id = c.root_chat_id;

COMMENT ON VIEW chats_with_acl IS
    'Projects each chat alongside its effective ACL. Sub-chats inherit the '
    'root chat''s ACL via COALESCE; orphaned sub-chats fall back to their own ACL.';

-- Add the chat:share scope to the api_key_scope enum.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat:share';

-- Three-state org setting for chat sharing: none | everyone | service_accounts.
CREATE TYPE shareable_chat_owners AS ENUM ('none', 'everyone', 'service_accounts');

ALTER TABLE organizations
    ADD COLUMN shareable_chat_owners shareable_chat_owners NOT NULL DEFAULT 'everyone';

COMMENT ON COLUMN organizations.shareable_chat_owners IS
    'Controls whose chats can be shared: none, everyone, or service_accounts.';
