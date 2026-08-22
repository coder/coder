-- An API key is held by an actor, and an actor is not always a user. The
-- holder becomes a (type, id) pair, which is how this corpus names a reference
-- standing where a foreign key into a union of identity tables would stand.
--
-- The foreign key to users goes with it. It could not survive a holder that is
-- not a user, and its ON DELETE CASCADE destroyed credentials without leaving
-- any record that they had ended.

ALTER TABLE api_keys DROP CONSTRAINT api_keys_user_id_uuid_fkey;

ALTER TABLE api_keys RENAME COLUMN user_id TO holder_id;

ALTER TABLE api_keys
    ADD COLUMN holder_type text NOT NULL DEFAULT 'user';

-- Text with a CHECK rather than an enum, deliberately. Extending an enum means
-- recreating the type, which on a table this size is a rewrite under lock.
ALTER TABLE api_keys
    ADD CONSTRAINT api_keys_holder_type_check
    CHECK (holder_type IN ('user', 'ai_agent'));

COMMENT ON COLUMN api_keys.holder_id IS 'The actor holding this key. Not a foreign key: the holder may be a user or an AI agent, and no single table holds both.';

COMMENT ON COLUMN api_keys.holder_type IS 'Which kind of actor holder_id names, and so which table resolves it.';

-- The trigger refusing a key for a deleted user predates holders that are not
-- users. It now applies only where the holder is one: for any other kind of
-- actor there is no users row to consult, and its absence is not a deletion.
CREATE OR REPLACE FUNCTION insert_apikey_fail_if_user_deleted() RETURNS trigger
    LANGUAGE plpgsql
    AS $$

DECLARE
BEGIN
	IF (NEW.holder_type = 'user' AND NEW.holder_id IS NOT NULL) THEN
		IF (SELECT deleted FROM users WHERE id = NEW.holder_id LIMIT 1) THEN
			RAISE EXCEPTION 'Cannot create API key for deleted user';
		END IF;
	END IF;
	RETURN NEW;
END;
$$;

-- Soft deleting a user removes their keys by trigger. The predicate has to
-- name the holder, and has to say that the holder is a user: an AI agent's key
-- is not swept up by the deletion of some unrelated person who happens to
-- share no relationship with it.
--
-- This is the destruction that leaves no record, now visible in one more
-- place. It is left in place for this change, which is about the holder rather
-- than about the credential lifecycle.
CREATE OR REPLACE FUNCTION delete_deleted_user_resources() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
BEGIN
    IF (NEW.deleted) THEN
        -- Remove their api_keys.
        DELETE FROM api_keys
        WHERE holder_type = 'user' AND holder_id = OLD.id;

        -- Remove their user_links.
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

        -- Remove their user AI provider keys.
        -- user_ai_provider_keys.user_id has ON DELETE CASCADE, but soft-delete
        -- does not remove the users row so the FK cascade never fires.
        DELETE FROM user_ai_provider_keys
        WHERE user_id = OLD.id;

        -- Remove their organization memberships.
        -- This also triggers group membership cleanup via
        -- trigger_delete_group_members_on_org_member_delete.
        DELETE FROM organization_members
        WHERE user_id = OLD.id;

        -- Remove their user_skills.
        -- user_skills.user_id has ON DELETE CASCADE, but soft-delete
        -- does not remove the users row so the FK cascade never fires.
        DELETE FROM user_skills
        WHERE user_id = OLD.id;
    END IF;
    RETURN NEW;
END;
$$;
