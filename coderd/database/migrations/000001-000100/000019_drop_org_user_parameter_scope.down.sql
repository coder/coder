CREATE TYPE old_parameter_scope AS ENUM (
	'organization',
	'template',
	'import_job',
	'user',
	'workspace'
	);
ALTER TABLE parameter_values ALTER COLUMN scope TYPE old_parameter_scope USING (scope::text::old_parameter_scope);
DROP TYPE parameter_scope;
ALTER TYPE old_parameter_scope RENAME TO parameter_scope;
