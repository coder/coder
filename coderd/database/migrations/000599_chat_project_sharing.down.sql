-- No-op for the enum value, enum values can't be dropped.

DELETE FROM notification_templates WHERE id = '3f1c6d8a-5b2e-4f7a-9c0d-7e8b2a4c6d1f';

ALTER TABLE chat_projects
    DROP CONSTRAINT IF EXISTS chat_projects_group_acl_is_object,
    DROP CONSTRAINT IF EXISTS chat_projects_user_acl_is_object,
    DROP COLUMN IF EXISTS group_acl,
    DROP COLUMN IF EXISTS user_acl;
