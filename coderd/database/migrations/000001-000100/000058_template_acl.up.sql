ALTER TABLE templates ADD COLUMN user_acl jsonb NOT NULL default '{}';
ALTER TABLE templates ADD COLUMN group_acl jsonb NOT NULL default '{}';

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
	FOREIGN KEY(group_id) REFERENCES groups(id) ON DELETE CASCADE,
	UNIQUE(user_id, group_id)
);

-- Insert a group for every organization (which should just be 1).
INSERT INTO groups (
	id,
	name,
	organization_id
) SELECT
	id, 'Everyone' as name, id
FROM
	organizations;

-- Insert allUsers groups into every existing template to avoid breaking
-- existing deployments.
UPDATE
	templates
SET
	group_acl = (
		SELECT
			json_build_object(
				organizations.id, array_to_json('{"read"}'::text[])
			)
		FROM
			organizations
		WHERE
			templates.organization_id = organizations.id
	);
