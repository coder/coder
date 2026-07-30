UPDATE
    templates
SET
    created_by = (
        SELECT user_id FROM organization_members
        WHERE organization_members.organization_id = templates.organization_id
        ORDER BY created_at
        LIMIT 1
    )
WHERE
    created_by IS NULL;


ALTER TABLE ONLY templates ALTER COLUMN created_by SET NOT NULL;
