-- Postgres cannot drop an enum value; 'chat:share' is left on api_key_scope.

ALTER TABLE chats DROP CONSTRAINT chat_acl_only_on_root_chats;
ALTER TABLE chats DROP CONSTRAINT chat_group_acl_not_null_jsonb;
ALTER TABLE chats DROP CONSTRAINT chat_user_acl_not_null_jsonb;
ALTER TABLE chats DROP COLUMN group_acl;
ALTER TABLE chats DROP COLUMN user_acl;
