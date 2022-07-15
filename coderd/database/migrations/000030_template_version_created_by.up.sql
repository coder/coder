BEGIN;

ALTER TABLE ONLY template_versions ADD COLUMN IF NOT EXISTS created_by uuid REFERENCES users (id) ON DELETE RESTRICT;

UPDATE
    template_versions
SET
    template_versions.created_by = (
        SELECT templates.created_by FROM templates
        WHERE template_versions.template_id = templates.id
        LIMIT 1
    )
WHERE
    template_versions.created_by IS NULL;

ALTER TABLE ONLY template_versions ALTER COLUMN created_by SET NOT NULL;

COMMIT;
