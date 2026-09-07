DROP TRIGGER IF EXISTS trigger_insert_group_members ON group_members;
DROP TRIGGER IF EXISTS trigger_insert_user_ai_budget_overrides ON user_ai_budget_overrides;
DROP TRIGGER IF EXISTS trigger_insert_organization_members ON organization_members;
DROP TRIGGER IF EXISTS trigger_insert_user_ai_provider_keys ON user_ai_provider_keys;
DROP TRIGGER IF EXISTS trigger_update_apikeys_owner ON api_keys;
DROP FUNCTION IF EXISTS fail_if_user_deleted();

-- Restore the standalone per-table guard bodies this migration replaced
-- (as reproduced from the pre-migration dump.sql).
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

CREATE OR REPLACE FUNCTION insert_user_links_fail_if_user_deleted() RETURNS trigger
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

CREATE OR REPLACE FUNCTION insert_user_secret_fail_if_user_deleted() RETURNS trigger
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

CREATE OR REPLACE FUNCTION insert_user_skill_fail_if_user_deleted() RETURNS trigger
    LANGUAGE plpgsql
    AS $$

BEGIN
    PERFORM 1
    FROM users
    WHERE id = NEW.user_id
      AND deleted = true
    LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION 'Cannot create user_skill for deleted user'
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'user_skill_user_deleted';
    END IF;
    RETURN NEW;
END;
$$;

DROP FUNCTION IF EXISTS check_user_not_deleted(uuid, boolean, text, text, text);
