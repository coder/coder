ALTER TABLE organizations ADD COLUMN deleted boolean DEFAULT FALSE NOT NULL;

DROP INDEX IF EXISTS idx_organization_name;
DROP INDEX IF EXISTS idx_organization_name_lower;

CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_name ON organizations USING btree (name)
	where deleted = false;
CREATE UNIQUE INDEX IF NOT EXISTS idx_organization_name_lower ON organizations USING btree (lower(name))
	where deleted = false;

ALTER TABLE ONLY organizations
	DROP CONSTRAINT IF EXISTS organizations_name;

CREATE FUNCTION protect_provisioned_organizations()
	RETURNS TRIGGER AS
$$
DECLARE
    workspace_count int;
	template_count int;
BEGIN
    workspace_count := (
        SELECT count(*) as count FROM workspaces
        WHERE
            workspaces.organization_id = OLD.id
            AND workspaces.deleted = false
    );

	template_count := (
        SELECT count(*) as count FROM templates
        WHERE
            templates.organization_id = OLD.id
            AND templates.deleted = false
    );

    -- Fail the deletion if one of the following:
    -- * the organization has 1 or more workspaces
	-- * the organization has 1 or more templates
    IF (workspace_count + template_count) > 0 THEN
            RAISE EXCEPTION 'cannot delete organization: organization has % workspaces and % templates that must be deleted first', workspace_count, template_count;
    END IF;

    -- add more cases to fail a delete

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

-- Trigger to protect organizations from being soft deleted with existing resources
CREATE TRIGGER protect_provisioned_organizations
    BEFORE UPDATE ON organizations
    FOR EACH ROW
	WHEN (NEW.deleted = true AND OLD.deleted = false)
    EXECUTE FUNCTION protect_provisioned_organizations();
