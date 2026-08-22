DROP TRIGGER IF EXISTS protect_deleting_organizations ON organizations;

-- Replace the function with the new implementation
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
        SELECT
            count(*) AS count
        FROM
            organization_members
        LEFT JOIN users ON users.id = organization_members.user_id
        WHERE
            organization_members.organization_id = OLD.id
            AND users.deleted = FALSE
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

    -- Only create error message for resources that actually exist
    IF (workspace_count + template_count + provisioner_keys_count) > 0 THEN
        DECLARE
            error_message text := 'cannot delete organization: organization has ';
            error_parts text[] := '{}';
        BEGIN
            IF workspace_count > 0 THEN
                error_parts := array_append(error_parts, workspace_count || ' workspaces');
            END IF;
            
            IF template_count > 0 THEN
                error_parts := array_append(error_parts, template_count || ' templates');
            END IF;
            
            IF provisioner_keys_count > 0 THEN
                error_parts := array_append(error_parts, provisioner_keys_count || ' provisioner keys');
            END IF;
            
            error_message := error_message || array_to_string(error_parts, ', ') || ' that must be deleted first';
            RAISE EXCEPTION '%', error_message;
        END;
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

-- Trigger to protect organizations from being soft deleted with existing resources
CREATE TRIGGER protect_deleting_organizations
    BEFORE UPDATE ON organizations
    FOR EACH ROW
    WHEN (NEW.deleted = true AND OLD.deleted = false)
    EXECUTE FUNCTION protect_deleting_organizations();
