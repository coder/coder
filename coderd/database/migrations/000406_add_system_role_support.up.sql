-- Add is_system column to identify system-managed roles.
ALTER TABLE custom_roles
    ADD COLUMN is_system boolean NOT NULL DEFAULT false;

-- Add member_permissions column for member-scoped permissions within an organization.
ALTER TABLE custom_roles
    ADD COLUMN member_permissions jsonb NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN custom_roles.is_system IS
    'System roles are managed by Coder and cannot be modified or deleted by users.';
