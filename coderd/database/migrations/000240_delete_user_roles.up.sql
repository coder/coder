-- When a custom role is deleted, we need to remove the assigned role
-- from all organization members that have it.
-- This action cannot be reverted, so deleting a custom role should be
-- done with caution.
CREATE OR REPLACE FUNCTION remove_organization_member_role()
	RETURNS TRIGGER AS
$$
BEGIN
	-- Delete the role from all organization members that have it.
	-- TODO: When site wide custom roles are supported, if the
	--	organization_id is null, we should remove the role from the 'users'
	--	table instead.
	IF OLD.organization_id IS NOT NULL THEN
		UPDATE organization_members
		-- this is a noop if the role is not assigned to the member
		SET roles = array_remove(roles, OLD.name)
		WHERE
			-- Scope to the correct organization
			organization_members.organization_id = OLD.organization_id;
	END IF;
	RETURN OLD;
END;
$$ LANGUAGE plpgsql;


-- Attach the function to deleting the custom role
CREATE TRIGGER remove_organization_member_custom_role
	BEFORE DELETE ON custom_roles FOR EACH ROW
	EXECUTE PROCEDURE remove_organization_member_role();


COMMENT ON TRIGGER
	remove_organization_member_custom_role
	ON custom_roles IS
		'When a custom_role is deleted, this trigger removes the role from all organization members.';
