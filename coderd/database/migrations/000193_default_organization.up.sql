-- This migration is intended to maintain the existing behavior of single org
-- deployments, while allowing for multi-org deployments. By default, this organization
-- will be used when no organization is specified.
ALTER TABLE organizations ADD COLUMN is_default BOOLEAN NOT NULL DEFAULT FALSE;

-- Only 1 org should ever be set to is_default.
create unique index organizations_single_default_org on organizations (is_default)
	where is_default = true;

UPDATE
	organizations
SET
	is_default = true
WHERE
	-- The first organization created will be the default.
	id = (SELECT id FROM organizations ORDER BY organizations.created_at ASC LIMIT 1 );
