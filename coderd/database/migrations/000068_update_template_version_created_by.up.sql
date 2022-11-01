BEGIN;

UPDATE
    template_versions
SET
    created_by = COALESCE(
        -- Best effort to convert all unowned template versions to the first owner.
        (SELECT id FROM users WHERE rbac_roles @> '{owner}' AND deleted = 'f' ORDER BY created_at ASC LIMIT 1),
        -- If there are no owners, assign to the first user.
        (SELECT id FROM users WHERE deleted = 'f' ORDER BY created_at ASC LIMIT 1)
    )
WHERE
    created_by IS NULL;

ALTER TABLE template_versions ALTER COLUMN created_by SET NOT NULL;

COMMIT;
