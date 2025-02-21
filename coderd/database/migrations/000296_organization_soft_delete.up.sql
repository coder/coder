ALTER TABLE organizations ADD COLUMN deleted boolean DEFAULT FALSE NOT NULL;

DROP INDEX IF EXISTS idx_organization_name;
DROP INDEX IF EXISTS idx_organization_name_lower;

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
	group_count int;
	member_count int;
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

	group_count := (
        SELECT count(*) as count FROM groups
        WHERE
            groups.organization_id = OLD.id
    );

	member_count := (
        SELECT count(*) as count FROM organization_members
        WHERE
            organization_members.organization_id = OLD.id
    );

    -- Fail the deletion if one of the following:
    -- * the organization has 1 or more workspaces
	-- * the organization has 1 or more templates
	-- * the organization has 1 or more groups other than "Everyone" group
	-- * the organization has 1 or more members other than the organization owner

    IF (workspace_count + template_count) > 0 THEN
            RAISE EXCEPTION 'cannot delete organization: organization has % workspaces and % templates that must be deleted first', workspace_count, template_count;
    END IF;

	IF (group_count + member_count) > 2 THEN
            RAISE EXCEPTION 'cannot delete organization: organization has % groups and % members that must be deleted first', group_count - 1, member_count - 1;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to protect organizations from being soft deleted with existing resources
CREATE TRIGGER protect_provisioned_organizations
    BEFORE UPDATE ON organizations
    FOR EACH ROW
	WHEN (NEW.deleted = true AND OLD.deleted = false)
    EXECUTE FUNCTION protect_provisioned_organizations();
