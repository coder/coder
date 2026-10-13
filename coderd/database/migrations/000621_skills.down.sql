-- resource_type and api_key_scope enum values cannot be dropped.

-- Only personal skills fit the user_skills shape.
DELETE FROM skills WHERE user_id IS NULL;

DROP TRIGGER trigger_delete_skill_user_acl_on_org_member_delete ON organization_members;
DROP FUNCTION delete_skill_user_acl_on_org_member_delete();

DROP TRIGGER trigger_upsert_skills ON skills;
DROP TRIGGER trigger_skills_per_owner_limit ON skills;
DROP FUNCTION enforce_skills_per_owner_limit();

DROP INDEX skills_project_id_name_idx;
DROP INDEX skills_organization_id_name_idx;

ALTER TABLE skills
    DROP CONSTRAINT skills_acl_only_on_organization_skills,
    DROP CONSTRAINT skills_user_acl_is_object,
    DROP CONSTRAINT skills_group_acl_is_object,
    DROP CONSTRAINT skills_single_owner,
    DROP COLUMN user_acl,
    DROP COLUMN group_acl,
    DROP COLUMN enabled,
    DROP COLUMN project_id,
    DROP COLUMN organization_id,
    ALTER COLUMN user_id SET NOT NULL;

ALTER INDEX skills_user_id_name_idx RENAME TO user_skills_user_id_name_idx;

ALTER TABLE skills RENAME CONSTRAINT skills_content_size TO user_skills_content_size;
ALTER TABLE skills RENAME CONSTRAINT skills_description_size TO user_skills_description_size;
ALTER TABLE skills RENAME CONSTRAINT skills_name_format TO user_skills_name_format;
ALTER TABLE skills RENAME CONSTRAINT skills_name_size TO user_skills_name_size;
ALTER TABLE skills RENAME CONSTRAINT skills_user_id_fkey TO user_skills_user_id_fkey;
ALTER TABLE skills RENAME CONSTRAINT skills_pkey TO user_skills_pkey;

ALTER TABLE skills RENAME TO user_skills;

CREATE FUNCTION enforce_user_skills_per_user_limit() RETURNS trigger
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

CREATE TRIGGER trigger_user_skills_per_user_limit
    BEFORE INSERT ON user_skills
    FOR EACH ROW
    EXECUTE FUNCTION enforce_user_skills_per_user_limit();

CREATE TRIGGER trigger_upsert_user_skills
    BEFORE INSERT OR UPDATE ON user_skills
    FOR EACH ROW
    EXECUTE FUNCTION insert_user_skill_fail_if_user_deleted();

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
