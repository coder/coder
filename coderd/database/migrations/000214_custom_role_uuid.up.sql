-- (name) is the primary key, this column is almost exclusively for auditing.
ALTER TABLE custom_roles ADD COLUMN id uuid DEFAULT gen_random_uuid() NOT NULL;
-- Ensure unique uuids.
CREATE INDEX idx_custom_roles_id ON custom_roles (id);
