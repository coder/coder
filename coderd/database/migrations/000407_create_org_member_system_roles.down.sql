-- Remove organization-member system roles created by the up migration.
DELETE FROM
    custom_roles
WHERE
    name = 'organization-member'
    AND is_system = true;
