CREATE FUNCTION delete_group_members_on_org_member_delete() RETURNS TRIGGER
	LANGUAGE plpgsql
AS $$
DECLARE
BEGIN
	-- Remove the user from all groups associated with the same
	-- organization as the organization_member being deleted.
	DELETE FROM group_members
	WHERE
		user_id = OLD.user_id
		AND group_id IN (
			SELECT id
			FROM groups
			WHERE organization_id = OLD.organization_id
		);
	RETURN OLD;
END;
$$;

CREATE TRIGGER trigger_delete_group_members_on_org_member_delete
	BEFORE DELETE ON organization_members
	FOR EACH ROW
EXECUTE PROCEDURE delete_group_members_on_org_member_delete();
