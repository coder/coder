ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_project:share';

ALTER TABLE chat_projects
    ADD COLUMN user_acl jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN group_acl jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE chat_projects
    ADD CONSTRAINT chat_projects_user_acl_is_object
        CHECK (jsonb_typeof(user_acl) = 'object'),
    ADD CONSTRAINT chat_projects_group_acl_is_object
        CHECK (jsonb_typeof(group_acl) = 'object');

COMMENT ON COLUMN chat_projects.user_acl IS 'Users the project is shared with, keyed by user ID.';
COMMENT ON COLUMN chat_projects.group_acl IS 'Groups the project is shared with, keyed by group ID. The organization ID is the Everyone group.';
