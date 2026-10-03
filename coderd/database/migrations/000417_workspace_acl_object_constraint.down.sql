ALTER TABLE workspaces
    DROP CONSTRAINT IF EXISTS group_acl_is_object,
    DROP CONSTRAINT IF EXISTS user_acl_is_object;
