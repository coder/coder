DROP INDEX templates_organization_id_name_idx;
CREATE UNIQUE INDEX templates_organization_id_name_idx ON templates USING btree (organization_id, name) WHERE deleted = false;
CREATE UNIQUE INDEX idx_templates_name_lower ON templates USING btree (lower(name));

ALTER TABLE ONLY templates ADD CONSTRAINT templates_organization_id_name_key UNIQUE (organization_id, name);
