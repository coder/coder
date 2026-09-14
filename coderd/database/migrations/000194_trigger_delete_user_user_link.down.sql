DROP TRIGGER IF EXISTS trigger_update_users ON users;
DROP FUNCTION IF EXISTS delete_deleted_user_resources;

DROP TRIGGER IF EXISTS trigger_upsert_user_links ON user_links;
DROP FUNCTION IF EXISTS insert_user_links_fail_if_user_deleted;

-- Restore the previous trigger
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
