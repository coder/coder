-- Drop the BEFORE INSERT/UPDATE guard added by 000489.
DROP TRIGGER IF EXISTS trigger_upsert_user_secrets ON user_secrets;
DROP FUNCTION IF EXISTS insert_user_secret_fail_if_user_deleted;

-- Restore the previous body of delete_deleted_user_resources() from
-- 000194_trigger_delete_user_user_link.up.sql, dropping the
-- user_secrets cleanup added by 000489.
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
	END IF;
	RETURN NEW;
END;
$$;
