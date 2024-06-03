-- (name) is the primary key, this column is almost exclusively for auditing.
ALTER TABLE custom_roles ADD COLUMN id uuid DEFAULT gen_random_uuid() NOT NULL;
-- Previous custom roles are now invalid, as the json changed. Since this is an
-- experimental feature, there is no point in trying to save the perms.
-- This does not elevate any permissions, so it is not a security issue.
UPDATE custom_roles SET org_permissions = '[]';

-- Ensure unique uuids.
CREATE INDEX idx_custom_roles_id ON custom_roles (id);
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'custom_role';
