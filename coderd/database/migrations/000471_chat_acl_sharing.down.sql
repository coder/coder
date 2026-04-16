-- Reverse 000471_chat_acl_sharing in reverse order.

ALTER TABLE organizations DROP COLUMN shareable_chat_owners;
DROP TYPE shareable_chat_owners;

-- Postgres cannot drop an enum value, so we mirror the no-op posture
-- of 000384_add_workspace_share_scope.down.sql rather than rebuild
-- api_key_scope. The 'chat:share' value is retained; nothing references
-- it once the feature is rolled back. If strict removal is ever
-- required, it should be a separate, deliberate migration that rebuilds
-- the enum type and casts every column through text, consistent with
-- whatever the canonical enum-rebuild pattern is at that time.

DROP VIEW chats_with_acl;

ALTER TABLE chats DROP CONSTRAINT chat_group_acl_not_null_jsonb;
ALTER TABLE chats DROP CONSTRAINT chat_user_acl_not_null_jsonb;
ALTER TABLE chats DROP COLUMN group_acl;
ALTER TABLE chats DROP COLUMN user_acl;
