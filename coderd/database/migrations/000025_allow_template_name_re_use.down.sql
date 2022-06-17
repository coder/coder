DROP INDEX idx_templates_name_lower;
CREATE UNIQUE INDEX idx_templates_name_lower ON templates USING btree (lower(name));

ALTER TABLE ONLY templates ADD CONSTRAINT templates_organization_id_name_key UNIQUE (organization_id, name);
