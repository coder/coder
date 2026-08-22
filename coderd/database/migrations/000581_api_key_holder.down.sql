ALTER TABLE api_keys DROP CONSTRAINT api_keys_holder_type_check;

ALTER TABLE api_keys DROP COLUMN holder_type;

ALTER TABLE api_keys RENAME COLUMN holder_id TO user_id;

CREATE OR REPLACE FUNCTION insert_apikey_fail_if_user_deleted() RETURNS trigger
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

-- Restoring the foreign key requires every holder to be a user. Any key held
-- by another kind of actor must be removed first, since there is nowhere for
-- it to point.
DELETE FROM api_keys
WHERE user_id NOT IN (SELECT id FROM users);

ALTER TABLE api_keys
    ADD CONSTRAINT api_keys_user_id_uuid_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
