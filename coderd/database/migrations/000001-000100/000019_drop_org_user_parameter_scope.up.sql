-- We no longer support org or user scoped values, so delete them
DELETE FROM parameter_values WHERE scope IN ('organization', 'user');

CREATE TYPE new_parameter_scope AS ENUM (
     'template',
     'import_job',
     'workspace'
);
ALTER TABLE parameter_values ALTER COLUMN scope TYPE new_parameter_scope USING (scope::text::new_parameter_scope);
DROP TYPE parameter_scope;
ALTER TYPE new_parameter_scope RENAME TO parameter_scope;
