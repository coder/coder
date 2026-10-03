-- Extend the soft-delete cleanup trigger to also wipe user_secrets.
-- user_secrets.user_id has ON DELETE CASCADE, but Coder soft-deletes
-- users by flipping users.deleted instead of removing the row, so the
-- FK cascade never fires and secrets would otherwise survive deletion.
--
-- Backfill any rows that belonged to already-soft-deleted users before
-- replacing the function.
DELETE FROM
	user_secrets
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
	END IF;
	RETURN NEW;
END;
$$;

-- Prevent adding new user_secrets for soft-deleted users.
-- Closes the window between an in-flight CreateUserSecret request
-- and the soft-delete UPDATE committing.
CREATE FUNCTION insert_user_secret_fail_if_user_deleted() RETURNS trigger
	LANGUAGE plpgsql
AS $$

DECLARE
BEGIN
	IF (NEW.user_id IS NOT NULL) THEN
		IF (SELECT deleted FROM users WHERE id = NEW.user_id LIMIT 1) THEN
			RAISE EXCEPTION 'Cannot create user_secret for deleted user';
		END IF;
	END IF;
	RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_upsert_user_secrets
	BEFORE INSERT OR UPDATE ON user_secrets
	FOR EACH ROW
EXECUTE PROCEDURE insert_user_secret_fail_if_user_deleted();
