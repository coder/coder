
ALTER TABLE ONLY template_versions ADD COLUMN IF NOT EXISTS created_by uuid REFERENCES users (id) ON DELETE RESTRICT;

UPDATE
    template_versions
SET
    created_by = (
        SELECT created_by FROM templates
        WHERE template_versions.template_id = templates.id
        LIMIT 1
    )
WHERE
    created_by IS NULL;
