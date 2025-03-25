-- Drop trigger that uses this function
DROP TRIGGER IF EXISTS protect_deleting_organizations ON organizations;

-- Revert the function to its original implementation
CREATE OR REPLACE FUNCTION protect_deleting_organizations()
    RETURNS TRIGGER AS
$$
DECLARE
    workspace_count int;
    template_count int;
    group_count int;
    member_count int;
    provisioner_keys_count int;
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

    provisioner_keys_count := (
        Select count(*) as count FROM provisioner_keys
        WHERE
            provisioner_keys.organization_id = OLD.id
    );

    -- Fail the deletion if one of the following:
    -- * the organization has 1 or more workspaces
    -- * the organization has 1 or more templates
    -- * the organization has 1 or more groups other than "Everyone" group
    -- * the organization has 1 or more members other than the organization owner
    -- * the organization has 1 or more provisioner keys

    IF (workspace_count + template_count + provisioner_keys_count) > 0 THEN
            RAISE EXCEPTION 'cannot delete organization: organization has % workspaces, % templates, and % provisioner keys that must be deleted first', workspace_count, template_count, provisioner_keys_count;
    END IF;

    IF (group_count) > 1 THEN
            RAISE EXCEPTION 'cannot delete organization: organization has % groups that must be deleted first', group_count - 1;
    END IF;

    -- Allow 1 member to exist, because you cannot remove yourself. You can
    -- remove everyone else. Ideally, we only omit the member that matches
    -- the user_id of the caller, however in a trigger, the caller is unknown.
    IF (member_count) > 1 THEN
            RAISE EXCEPTION 'cannot delete organization: organization has % members that must be deleted first', member_count - 1;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Re-create trigger that uses this function
CREATE TRIGGER protect_deleting_organizations
    BEFORE DELETE ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION protect_deleting_organizations();
