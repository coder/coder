-- Add user/group ACL columns to chats so owners can share a chat
-- read-only with specific users and groups. Mirrors
-- workspaces.user_acl / workspaces.group_acl from migration 000354
-- (ACL storage) plus 000417 (not-null/jsonb-object constraints).
ALTER TABLE chats
    ADD COLUMN user_acl  jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN group_acl jsonb NOT NULL DEFAULT '{}'::jsonb;

-- Reject NULL jsonb objects at the column level so downstream views
-- and Rego->SQL compilation can treat the column as a map.
ALTER TABLE chats
    ADD CONSTRAINT chat_user_acl_not_null_jsonb
        CHECK (user_acl IS NOT NULL AND jsonb_typeof(user_acl) = 'object'),
    ADD CONSTRAINT chat_group_acl_not_null_jsonb
        CHECK (group_acl IS NOT NULL AND jsonb_typeof(group_acl) = 'object');

-- chats_with_acl projects each chat alongside its EFFECTIVE ACL. The
-- view's user_acl and group_acl columns are the root chat's ACL for
-- sub-chats (via LEFT JOIN + COALESCE) and the chat's own ACL for
-- root chats. Keeping the column names identical to the base table
-- means sqlc-generated row types stay compatible with database.Chat
-- and callers do not need a second Go type. Writes continue to target
-- the chats table directly; the view is read-only. If a root chat is
-- hard-deleted, ON DELETE SET NULL collapses root_chat_id to NULL on
-- descendants and the LEFT JOIN misses, so orphaned sub-chats fall
-- back to their own (empty) ACL by design.
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
    'root chat''s user_acl / group_acl via LEFT JOIN + COALESCE. Orphaned '
    'sub-chats (root_chat_id IS NULL after a root delete) fall back to the '
    'descendant''s own ACL. Column names match the base chats table so '
    'sqlc row types are shared.';

-- Add the chat:share scope to the api_key_scope enum. Mirrors
-- migration 000384 for workspace:share. Postgres requires enum value
-- additions to run outside the enclosing transaction, which the
-- migration runner already handles for this file pattern.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat:share';

-- Three-state org setting for chat sharing: none | everyone |
-- service_accounts. Default is 'everyone' to preserve existing
-- behaviour; the enterprise org-settings handler (added in the
-- accompanying Go change) is how operators tighten this.
-- Mirrors migration 000443 which did the same for workspaces.
CREATE TYPE shareable_chat_owners AS ENUM ('none', 'everyone', 'service_accounts');

ALTER TABLE organizations
    ADD COLUMN shareable_chat_owners shareable_chat_owners NOT NULL DEFAULT 'everyone';

COMMENT ON COLUMN organizations.shareable_chat_owners IS
    'Controls whose chats can be shared: none, everyone, or service_accounts.';
