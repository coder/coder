-- It's not possible to drop enum values from enum types, so the UP has "IF NOT
-- EXISTS".

-- Delete all jobs that use the new enum value.
DELETE FROM
    provisioner_jobs
WHERE
    type = 'template_version_dry_run';
