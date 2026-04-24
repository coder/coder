ALTER TABLE chats
    ADD COLUMN user_acl  jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN group_acl jsonb NOT NULL DEFAULT '{}'::jsonb;

-- Enforce jsonb-object shape so downstream views and Rego->SQL treat the
-- columns as maps. Keep ACL entries on root chats only, child chats inherit
-- sharing from the root.
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

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat:share';
