-- Restore the previous body of delete_deleted_user_resources() from
-- migration 000490 (without the organization_members cleanup).
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
	END IF;
	RETURN NEW;
END;
$$;
