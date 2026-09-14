-- We need to delete all existing API keys for soft-deleted users.
DELETE FROM
	api_keys
WHERE
	user_id
IN (
	SELECT id FROM users WHERE deleted
);


-- When we soft-delete a user, we also want to delete their API key.
CREATE FUNCTION delete_deleted_user_api_keys() RETURNS trigger
	LANGUAGE plpgsql
	AS $$
DECLARE
BEGIN
	IF (NEW.deleted) THEN
		DELETE FROM api_keys
		WHERE user_id = OLD.id;
	END IF;
	RETURN NEW;
END;
$$;


CREATE TRIGGER trigger_update_users
AFTER INSERT OR UPDATE ON users
FOR EACH ROW
WHEN (NEW.deleted = true)
EXECUTE PROCEDURE delete_deleted_user_api_keys();

-- When we insert a new api key, we want to fail if the user is soft-deleted.
CREATE FUNCTION insert_apikey_fail_if_user_deleted() RETURNS trigger
	LANGUAGE plpgsql
	AS $$

DECLARE
BEGIN
	IF (NEW.user_id IS NOT NULL) THEN
		IF (SELECT deleted FROM users WHERE id = NEW.user_id LIMIT 1) THEN
			RAISE EXCEPTION 'Cannot create API key for deleted user';
		END IF;
	END IF;
	RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_insert_apikeys
BEFORE INSERT ON api_keys
FOR EACH ROW
EXECUTE PROCEDURE insert_apikey_fail_if_user_deleted();
