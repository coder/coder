BEGIN;

ALTER TABLE templates ADD COLUMN user_acl jsonb NOT NULL default '{}';
ALTER TABLE templates ADD COLUMN is_private boolean NOT NULL default 'false';

CREATE TYPE template_role AS ENUM (
	'read',
	'write',
	'admin'
);

COMMIT;
