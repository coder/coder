DROP INDEX idx_templates_name_lower;
CREATE UNIQUE INDEX idx_templates_name_lower ON templates USING btree (lower(name)) WHERE deleted = false;

ALTER TABLE ONLY templates DROP CONSTRAINT templates_organization_id_name_key;
