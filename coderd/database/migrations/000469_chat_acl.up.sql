ALTER TABLE chats
    ADD COLUMN user_acl jsonb DEFAULT '{}'::jsonb NOT NULL,
    ADD COLUMN group_acl jsonb DEFAULT '{}'::jsonb NOT NULL;

ALTER TABLE chats
    ADD CONSTRAINT chats_group_acl_is_object CHECK (jsonb_typeof(group_acl) = 'object'),
    ADD CONSTRAINT chats_user_acl_is_object CHECK (jsonb_typeof(user_acl) = 'object');

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat:share';
