-- Reverse 000471_chat_acl_sharing in reverse order.

ALTER TABLE organizations DROP COLUMN shareable_chat_owners;
DROP TYPE shareable_chat_owners;

-- Postgres cannot drop an enum value; leave 'chat:share' on api_key_scope.

DROP VIEW chats_with_acl;

ALTER TABLE chats DROP CONSTRAINT chat_group_acl_not_null_jsonb;
ALTER TABLE chats DROP CONSTRAINT chat_user_acl_not_null_jsonb;
ALTER TABLE chats DROP COLUMN group_acl;
ALTER TABLE chats DROP COLUMN user_acl;
