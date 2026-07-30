-- We need to delete all existing user_links for soft-deleted users
DELETE FROM
	user_links
WHERE
	user_id
		IN (
		SELECT id FROM users WHERE deleted
	);

-- Drop the old trigger
DROP TRIGGER trigger_update_users ON users;
-- Drop the old function
DROP FUNCTION delete_deleted_user_api_keys;

-- When we soft-delete a user, we also want to delete their API key.
-- The previous function deleted all api keys. This extends that with user_links.
CREATE FUNCTION delete_deleted_user_resources() RETURNS trigger
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


-- Update it to the new trigger
CREATE TRIGGER trigger_update_users
	AFTER INSERT OR UPDATE ON users
	FOR EACH ROW
	WHEN (NEW.deleted = true)
EXECUTE PROCEDURE delete_deleted_user_resources();


-- Prevent adding new user_links for soft-deleted users
CREATE FUNCTION insert_user_links_fail_if_user_deleted() RETURNS trigger
	LANGUAGE plpgsql
AS $$

DECLARE
BEGIN
	IF (NEW.user_id IS NOT NULL) THEN
		IF (SELECT deleted FROM users WHERE id = NEW.user_id LIMIT 1) THEN
			RAISE EXCEPTION 'Cannot create user_link for deleted user';
		END IF;
	END IF;
	RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_upsert_user_links
	BEFORE INSERT OR UPDATE ON user_links
	FOR EACH ROW
EXECUTE PROCEDURE insert_user_links_fail_if_user_deleted();
