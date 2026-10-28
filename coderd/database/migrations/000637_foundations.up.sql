CREATE TABLE foundations (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id),
    name text NOT NULL,
    provisioner_tags jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(provisioner_tags) = 'object'),
    template_id uuid UNIQUE,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    deleted boolean NOT NULL DEFAULT false,
    UNIQUE (id, organization_id),
    FOREIGN KEY (template_id, organization_id) REFERENCES templates(id, organization_id)
);
CREATE UNIQUE INDEX foundations_org_name_idx ON foundations(organization_id, lower(name)) WHERE NOT deleted;

-- Assign independent IDs while retaining revision, run, and workspace snapshot IDs.
-- Deleted templates can share a name with their replacement. Keep both roots
-- manageable, disambiguating names rather than discarding historical state.
DO $$
DECLARE
    t record;
    foundation_name text;
    suffix integer;
BEGIN
    FOR t IN
        SELECT templates.*, COALESCE(s.updated_at, templates.updated_at) AS resource_updated_at
        FROM templates LEFT JOIN template_resource_states s ON s.template_id = templates.id
        WHERE templates.id IN (
            SELECT template_id FROM template_resource_states
            UNION SELECT template_id FROM template_resource_revisions
            UNION SELECT template_id FROM template_resource_runs
        )
        ORDER BY templates.deleted, templates.id
    LOOP
        foundation_name := t.name;
        suffix := 0;
        WHILE EXISTS (SELECT 1 FROM foundations WHERE organization_id = t.organization_id AND lower(name) = lower(foundation_name)) LOOP
            suffix := suffix + 1;
            foundation_name := rtrim(left(t.name, 31 - length(suffix::text)), '-') || '-' || suffix::text;
        END LOOP;
        INSERT INTO foundations (id, organization_id, name, template_id, created_at, updated_at)
        VALUES (gen_random_uuid(), t.organization_id, foundation_name, t.id, t.resource_updated_at, t.resource_updated_at);
    END LOOP;
END $$;

-- Remove ownership constraints before replacing the referenced IDs. Other
-- constraints, including workspace snapshots and job organization, remain intact.
DO $$
DECLARE c record;
BEGIN
    FOR c IN SELECT conrelid::regclass AS tbl, conname FROM pg_constraint
        WHERE contype = 'f' AND conrelid IN ('template_resource_states'::regclass, 'template_resource_revisions'::regclass, 'template_resource_runs'::regclass)
        AND confrelid IN ('templates'::regclass, 'template_resource_revisions'::regclass, 'template_resource_runs'::regclass)
    LOOP
        EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', c.tbl, c.conname);
    END LOOP;
END $$;

UPDATE template_resource_states s SET template_id = f.id FROM foundations f WHERE f.template_id = s.template_id;
UPDATE template_resource_revisions r SET template_id = f.id FROM foundations f WHERE f.template_id = r.template_id;
UPDATE template_resource_runs r SET template_id = f.id FROM foundations f WHERE f.template_id = r.template_id;
ALTER TABLE template_resource_states RENAME COLUMN template_id TO foundation_id;
ALTER TABLE template_resource_revisions RENAME COLUMN template_id TO foundation_id;
ALTER TABLE template_resource_runs RENAME COLUMN template_id TO foundation_id;

ALTER TABLE template_resource_states
    ADD FOREIGN KEY (foundation_id, organization_id) REFERENCES foundations(id, organization_id),
    ADD FOREIGN KEY (current_revision_id, foundation_id, organization_id) REFERENCES template_resource_revisions(id, foundation_id, organization_id),
    ADD FOREIGN KEY (applied_revision_id, foundation_id, organization_id) REFERENCES template_resource_revisions(id, foundation_id, organization_id),
    ADD FOREIGN KEY (active_run_id, foundation_id, organization_id) REFERENCES template_resource_runs(id, foundation_id, organization_id);
ALTER TABLE template_resource_revisions ADD FOREIGN KEY (foundation_id, organization_id) REFERENCES foundations(id, organization_id);
ALTER TABLE template_resource_runs
    ADD FOREIGN KEY (foundation_id, organization_id) REFERENCES foundations(id, organization_id),
    ADD FOREIGN KEY (revision_id, foundation_id, organization_id) REFERENCES template_resource_revisions(id, foundation_id, organization_id);

ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'foundation';
