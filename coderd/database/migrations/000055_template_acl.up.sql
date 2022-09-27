BEGIN;

ALTER TABLE templates ADD COLUMN user_acl jsonb NOT NULL default '{}';
ALTER TABLE templates ADD COLUMN group_acl jsonb NOT NULL default '{}';

CREATE TYPE template_role AS ENUM (
	'read',
	'write',
	'admin'
);

CREATE TABLE groups (
	id uuid NOT NULL,
	name text NOT NULL,
	organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
	PRIMARY KEY(id),
	UNIQUE(name, organization_id)
);

CREATE TABLE group_members (
	user_id uuid NOT NULL,
	group_id uuid NOT NULL,
	FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
	FOREIGN KEY(group_id) REFERENCES groups(id) ON DELETE CASCADE
);

COMMIT;
