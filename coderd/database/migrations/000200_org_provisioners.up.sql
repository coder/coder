-- At the time of this migration, only 1 org is expected in a deployment.
-- In the future when multi-org is more common, there might be a use case
-- to allow a provisioner to be associated with multiple orgs.
ALTER TABLE provisioner_daemons
	ADD COLUMN organization_id UUID REFERENCES organizations(id) ON DELETE CASCADE;

UPDATE
	provisioner_daemons
SET
	-- Default to the first org
	organization_id = (SELECT id FROM organizations WHERE is_default = true LIMIT 1 );

ALTER TABLE provisioner_daemons
	ALTER COLUMN organization_id SET NOT NULL;
