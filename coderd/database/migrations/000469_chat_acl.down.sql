ALTER TABLE chats
    DROP CONSTRAINT IF EXISTS chats_group_acl_is_object,
    DROP CONSTRAINT IF EXISTS chats_user_acl_is_object;

ALTER TABLE chats
    DROP COLUMN IF EXISTS user_acl,
    DROP COLUMN IF EXISTS group_acl;
