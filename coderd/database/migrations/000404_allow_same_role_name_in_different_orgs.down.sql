-- Restore the original unique constraint (name only, no organization_id).
DROP INDEX IF EXISTS idx_custom_roles_name_lower_organization_id;

ALTER TABLE custom_roles DROP CONSTRAINT IF EXISTS organization_id_not_zero;

CREATE UNIQUE INDEX idx_custom_roles_name_lower ON custom_roles USING btree (LOWER(name));
