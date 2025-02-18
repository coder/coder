ALTER TABLE organizations ADD COLUMN deleted boolean DEFAULT FALSE NOT NULL;

DROP INDEX IF EXISTS idx_organization_name;
DROP INDEX IF EXISTS idx_organization_name_lower;

CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_name ON organizations USING btree (name)
	where deleted = false;
CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_name_lower ON organizations USING btree (lower(name))
	where deleted = false;

ALTER TABLE ONLY organizations
	DROP CONSTRAINT IF EXISTS organizations_name;
