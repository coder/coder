DELETE FROM skills WHERE user_id IS NULL;

DROP TRIGGER trigger_delete_skill_user_acl_on_org_member_delete ON organization_members;
DROP FUNCTION delete_skill_user_acl_on_org_member_delete();

DROP TRIGGER trigger_upsert_skills ON skills;
CREATE TRIGGER trigger_upsert_skills
    BEFORE INSERT OR UPDATE ON skills
    FOR EACH ROW
    EXECUTE FUNCTION insert_user_skill_fail_if_user_deleted();

DROP TRIGGER trigger_skills_per_owner_limit ON skills;
DROP FUNCTION enforce_skills_per_owner_limit();

CREATE FUNCTION enforce_skills_per_user_limit() RETURNS trigger
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
    FROM skills
    WHERE user_id = NEW.user_id;
    IF skill_count >= skill_limit THEN
        RAISE EXCEPTION 'user has reached the personal skill limit'
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'skills_per_user_limit';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_skills_per_user_limit
    BEFORE INSERT ON skills
    FOR EACH ROW
    EXECUTE FUNCTION enforce_skills_per_user_limit();

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
