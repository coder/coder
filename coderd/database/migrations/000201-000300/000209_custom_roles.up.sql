CREATE TABLE custom_roles (
	-- name is globally unique. Org scoped roles have their orgid appended
	-- like:  "name":"organization-admin:bbe8c156-c61e-4d36-b91e-697c6b1477e8"
	name text primary key,
	-- display_name is the actual name of the role displayed to the user.
	display_name text NOT NULL,

	-- Unfortunately these values are schemaless json documents.
	-- If there was a permission table for these, that would involve
	-- many necessary joins to accomplish this simple json.

	-- site_permissions is '[]Permission'
	site_permissions jsonb NOT NULL default '[]',
	-- org_permissions is 'map[<org_id>][]Permission'
	org_permissions jsonb NOT NULL default '{}',
	-- user_permissions is '[]Permission'
	user_permissions jsonb NOT NULL default '[]',

	-- extra convenience meta data.
	created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Ensure no case variants of the same roles
CREATE UNIQUE INDEX idx_custom_roles_name_lower ON custom_roles USING btree (lower(name));
COMMENT ON TABLE  custom_roles IS 'Custom roles allow dynamic roles expanded at runtime';
