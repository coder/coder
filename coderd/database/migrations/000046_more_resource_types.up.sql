CREATE TYPE new_resource_type AS ENUM (
    'organization',
    'template',
    'template_version',
    'user',
    'workspace',
    'git_ssh_key',
    'api_key'
);

ALTER TABLE audit_logs
	ALTER COLUMN resource_type TYPE new_resource_type USING(resource_type::text::new_resource_type);

DROP TYPE resource_type;
ALTER TYPE new_resource_type RENAME TO resource_type;
