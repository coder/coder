-- (name) is the primary key, this column is almost exclusively for auditing.
-- Audit logs require a uuid as the unique identifier for a resource.
ALTER TABLE custom_roles ADD COLUMN id uuid DEFAULT gen_random_uuid() NOT NULL;
COMMENT ON COLUMN custom_roles.id IS 'Custom roles ID is used purely for auditing purposes. Name is a better unique identifier.';

-- Ensure unique uuids.
CREATE INDEX idx_custom_roles_id ON custom_roles (id);
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'custom_role';
