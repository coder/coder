CREATE TYPE new_audit_action AS ENUM (
    'create',
    'write',
    'delete',
    'start',
    'stop'
);

CREATE TYPE new_resource_type AS ENUM (
    'organization',
    'template',
    'template_version',
    'user',
    'workspace',
    'git_ssh_key',
    'api_key',
    'group',
    'workspace_build'
);

ALTER TABLE audit_logs
	ALTER COLUMN action TYPE new_audit_action USING (action::text::new_audit_action),
	ALTER COLUMN resource_type TYPE new_resource_type USING (resource_type::text::new_resource_type);

DROP TYPE audit_action;
ALTER TYPE new_audit_action RENAME TO audit_action;
DROP TYPE resource_type;
ALTER TYPE new_resource_type RENAME TO resource_type;
