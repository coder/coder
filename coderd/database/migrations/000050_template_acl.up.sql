BEGIN;

ALTER TABLE templates ADD COLUMN user_acl jsonb NOT NULL default '{}';

CREATE TYPE template_role AS ENUM (
	'read',
	'write',
	'admin'
);

COMMIT;
