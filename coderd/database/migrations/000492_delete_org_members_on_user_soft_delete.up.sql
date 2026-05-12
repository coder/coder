-- Extend the soft-delete cleanup trigger to also remove organization_members.
-- organization_members.user_id has ON DELETE CASCADE, but Coder soft-deletes
-- users by flipping users.deleted instead of removing the row, so the
-- FK cascade never fires and memberships would otherwise survive deletion.
-- Removing an org membership also fires
-- trigger_delete_group_members_on_org_member_delete, which cleans up
-- the user's group memberships in that organization automatically.
--
-- Backfill any rows that belonged to already-soft-deleted users before
-- replacing the function.
DELETE FROM
	organization_members
WHERE
	user_id
		IN (
		SELECT id FROM users WHERE deleted
	);

CREATE OR REPLACE FUNCTION delete_deleted_user_resources() RETURNS trigger
	LANGUAGE plpgsql
AS $$
DECLARE
BEGIN
	IF (NEW.deleted) THEN
		-- Remove their api_keys
		DELETE FROM api_keys
		WHERE user_id = OLD.id;

		-- Remove their user_links
		-- Their login_type is preserved in the users table.
		-- Matching this user back to the link can still be done by their
		-- email if the account is undeleted. Although that is not a guarantee.
		DELETE FROM user_links
		WHERE user_id = OLD.id;

		-- Remove their user_secrets.
		-- user_secrets.user_id has ON DELETE CASCADE, but soft-delete
		-- does not remove the users row so the FK cascade never fires.
		DELETE FROM user_secrets
		WHERE user_id = OLD.id;

		-- Remove their organization memberships.
		-- This also triggers group membership cleanup via
		-- trigger_delete_group_members_on_org_member_delete.
		DELETE FROM organization_members
		WHERE user_id = OLD.id;
	END IF;
	RETURN NEW;
END;
$$;
