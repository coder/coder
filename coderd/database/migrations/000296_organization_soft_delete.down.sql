DROP INDEX IF EXISTS idx_organization_name_lower;

CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_name ON organizations USING btree (name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_name_lower ON organizations USING btree (lower(name));

ALTER TABLE ONLY organizations
	ADD CONSTRAINT organizations_name UNIQUE (name);

DROP TRIGGER IF EXISTS protect_deleting_organizations ON organizations;
DROP FUNCTION IF EXISTS protect_deleting_organizations;

ALTER TABLE organizations DROP COLUMN deleted;
