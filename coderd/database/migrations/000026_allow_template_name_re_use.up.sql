DROP INDEX idx_templates_name_lower;
DROP INDEX templates_organization_id_name_idx;
CREATE UNIQUE INDEX templates_organization_id_name_idx ON templates (organization_id, lower(name)) WHERE deleted = false;

ALTER TABLE ONLY templates DROP CONSTRAINT templates_organization_id_name_key;
