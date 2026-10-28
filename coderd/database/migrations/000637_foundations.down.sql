-- Downgrading cannot represent independent Foundation infrastructure safely.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM foundations WHERE template_id IS NULL) THEN
        RAISE EXCEPTION 'Bind all Foundations to templates before downgrading';
    END IF;
END $$;
DO $$
DECLARE c record;
BEGIN
    FOR c IN SELECT conrelid::regclass AS tbl, conname FROM pg_constraint
        WHERE contype = 'f' AND conrelid IN ('template_resource_states'::regclass, 'template_resource_revisions'::regclass, 'template_resource_runs'::regclass)
        AND confrelid IN ('foundations'::regclass, 'template_resource_revisions'::regclass, 'template_resource_runs'::regclass)
    LOOP
        EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', c.tbl, c.conname);
    END LOOP;
END $$;
UPDATE template_resource_states s SET foundation_id = f.template_id FROM foundations f WHERE f.id = s.foundation_id;
UPDATE template_resource_revisions r SET foundation_id = f.template_id FROM foundations f WHERE f.id = r.foundation_id;
UPDATE template_resource_runs r SET foundation_id = f.template_id FROM foundations f WHERE f.id = r.foundation_id;
ALTER TABLE template_resource_states RENAME COLUMN foundation_id TO template_id;
ALTER TABLE template_resource_revisions RENAME COLUMN foundation_id TO template_id;
ALTER TABLE template_resource_runs RENAME COLUMN foundation_id TO template_id;
ALTER TABLE template_resource_states
    ADD FOREIGN KEY (template_id, organization_id) REFERENCES templates(id, organization_id),
    ADD FOREIGN KEY (current_revision_id, template_id, organization_id) REFERENCES template_resource_revisions(id, template_id, organization_id),
    ADD FOREIGN KEY (applied_revision_id, template_id, organization_id) REFERENCES template_resource_revisions(id, template_id, organization_id),
    ADD FOREIGN KEY (active_run_id, template_id, organization_id) REFERENCES template_resource_runs(id, template_id, organization_id);
ALTER TABLE template_resource_revisions ADD FOREIGN KEY (template_id, organization_id) REFERENCES templates(id, organization_id);
ALTER TABLE template_resource_runs
    ADD FOREIGN KEY (template_id, organization_id) REFERENCES templates(id, organization_id),
    ADD FOREIGN KEY (revision_id, template_id, organization_id) REFERENCES template_resource_revisions(id, template_id, organization_id);
DROP TABLE foundations;
