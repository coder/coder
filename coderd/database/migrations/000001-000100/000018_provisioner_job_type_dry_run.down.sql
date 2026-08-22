-- It's not possible to drop enum values from enum types, so the UP has "IF NOT
-- EXISTS".

-- Delete all audit logs that use the new enum values.
DELETE FROM
    audit_logs
WHERE
    resource_type = 'git_ssh_key' OR
    resource_type = 'api_key';
