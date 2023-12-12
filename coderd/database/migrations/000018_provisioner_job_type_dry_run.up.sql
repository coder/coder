CREATE TYPE new_provisioner_job_type AS ENUM (
    'template_version_import',
    'workspace_build',
    'template_version_dry_run'
);

ALTER TABLE provisioner_jobs
	ALTER COLUMN "type" TYPE new_provisioner_job_type USING ("type"::text::new_provisioner_job_type);

DROP TYPE provisioner_job_type;
ALTER TYPE new_provisioner_job_type RENAME TO provisioner_job_type;
