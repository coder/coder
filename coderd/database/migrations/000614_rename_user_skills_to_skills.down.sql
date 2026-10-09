CREATE OR REPLACE FUNCTION delete_deleted_user_resources() RETURNS trigger
    LANGUAGE plpgsql
AS $$
DECLARE
BEGIN
    IF (NEW.deleted) THEN
        -- Remove their api_keys.
        DELETE FROM api_keys
        WHERE user_id = OLD.id;

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

CREATE OR REPLACE FUNCTION enforce_skills_per_user_limit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    skill_count int;
    skill_limit constant int := 100;
BEGIN
    -- Serialize skill-cap checks per user so concurrent inserts cannot all
    -- observe the same pre-insert count and exceed the hard limit.
    PERFORM 1
    FROM users
    WHERE id = NEW.user_id
    FOR UPDATE;

    SELECT count(*) INTO skill_count
    FROM user_skills
    WHERE user_id = NEW.user_id;
    IF skill_count >= skill_limit THEN
        RAISE EXCEPTION 'user has reached the personal skill limit'
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'user_skills_per_user_limit';
    END IF;
    RETURN NEW;
END;
$$;

ALTER FUNCTION enforce_skills_per_user_limit() RENAME TO enforce_user_skills_per_user_limit;

ALTER TRIGGER trigger_upsert_skills ON skills RENAME TO trigger_upsert_user_skills;
ALTER TRIGGER trigger_skills_per_user_limit ON skills RENAME TO trigger_user_skills_per_user_limit;

ALTER INDEX skills_user_id_name_idx RENAME TO user_skills_user_id_name_idx;

ALTER TABLE skills RENAME CONSTRAINT skills_content_size TO user_skills_content_size;
ALTER TABLE skills RENAME CONSTRAINT skills_description_size TO user_skills_description_size;
ALTER TABLE skills RENAME CONSTRAINT skills_name_format TO user_skills_name_format;
ALTER TABLE skills RENAME CONSTRAINT skills_name_size TO user_skills_name_size;
ALTER TABLE skills RENAME CONSTRAINT skills_user_id_fkey TO user_skills_user_id_fkey;
ALTER TABLE skills RENAME CONSTRAINT skills_pkey TO user_skills_pkey;

ALTER TABLE skills RENAME TO user_skills;
