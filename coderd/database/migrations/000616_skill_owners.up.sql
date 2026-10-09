ALTER TABLE skills
    ALTER COLUMN user_id DROP NOT NULL,
    ADD COLUMN organization_id uuid REFERENCES organizations(id) ON DELETE CASCADE,
    ADD COLUMN project_id uuid REFERENCES chat_projects(id) ON DELETE CASCADE,
    ADD COLUMN enabled boolean NOT NULL DEFAULT true,
    ADD COLUMN group_acl jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN user_acl jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD CONSTRAINT skills_single_owner CHECK (num_nonnulls(user_id, organization_id, project_id) = 1),
    ADD CONSTRAINT skills_group_acl_is_object CHECK (jsonb_typeof(group_acl) = 'object'),
    ADD CONSTRAINT skills_user_acl_is_object CHECK (jsonb_typeof(user_acl) = 'object'),
    ADD CONSTRAINT skills_acl_only_on_organization_skills CHECK (
        organization_id IS NOT NULL OR (group_acl = '{}'::jsonb AND user_acl = '{}'::jsonb)
    );

CREATE UNIQUE INDEX skills_organization_id_name_idx ON skills (organization_id, name) WHERE organization_id IS NOT NULL;
CREATE UNIQUE INDEX skills_project_id_name_idx ON skills (project_id, name) WHERE project_id IS NOT NULL;

DROP TRIGGER trigger_skills_per_user_limit ON skills;
DROP FUNCTION enforce_skills_per_user_limit();

CREATE FUNCTION enforce_skills_per_owner_limit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    skill_count int;
    skill_limit constant int := 100;
BEGIN
    -- Serialize skill-cap checks per owner so concurrent inserts cannot all
    -- observe the same pre-insert count and exceed the hard limit. FOR NO KEY
    -- UPDATE does not block the KEY SHARE locks of unrelated foreign keys.
    IF NEW.user_id IS NOT NULL THEN
        PERFORM 1 FROM users WHERE id = NEW.user_id FOR NO KEY UPDATE;
        SELECT count(*) INTO skill_count FROM skills WHERE user_id = NEW.user_id;
        IF skill_count >= skill_limit THEN
            RAISE EXCEPTION 'user has reached the personal skill limit'
                USING ERRCODE = 'check_violation',
                      CONSTRAINT = 'skills_per_user_limit';
        END IF;
    ELSIF NEW.organization_id IS NOT NULL THEN
        PERFORM 1 FROM organizations WHERE id = NEW.organization_id FOR NO KEY UPDATE;
        SELECT count(*) INTO skill_count FROM skills WHERE organization_id = NEW.organization_id;
        IF skill_count >= skill_limit THEN
            RAISE EXCEPTION 'organization has reached the skill limit'
                USING ERRCODE = 'check_violation',
                      CONSTRAINT = 'skills_per_organization_limit';
        END IF;
    ELSIF NEW.project_id IS NOT NULL THEN
        PERFORM 1 FROM chat_projects WHERE id = NEW.project_id FOR NO KEY UPDATE;
        SELECT count(*) INTO skill_count FROM skills WHERE project_id = NEW.project_id;
        IF skill_count >= skill_limit THEN
            RAISE EXCEPTION 'project has reached the skill limit'
                USING ERRCODE = 'check_violation',
                      CONSTRAINT = 'skills_per_project_limit';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_skills_per_owner_limit
    BEFORE INSERT ON skills
    FOR EACH ROW
    EXECUTE FUNCTION enforce_skills_per_owner_limit();

DROP TRIGGER trigger_upsert_skills ON skills;
CREATE TRIGGER trigger_upsert_skills
    BEFORE INSERT OR UPDATE ON skills
    FOR EACH ROW
    WHEN (NEW.user_id IS NOT NULL)
    EXECUTE FUNCTION insert_user_skill_fail_if_user_deleted();

CREATE FUNCTION delete_skill_user_acl_on_org_member_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    -- Drop the departing member's direct grants so adding them back to the
    -- organization does not silently restore access.
    UPDATE skills
    SET
        user_acl = user_acl - OLD.user_id::text,
        updated_at = now()
    WHERE organization_id = OLD.organization_id AND user_acl ? OLD.user_id::text;
    RETURN OLD;
END;
$$;

CREATE TRIGGER trigger_delete_skill_user_acl_on_org_member_delete
    BEFORE DELETE ON organization_members
    FOR EACH ROW
    EXECUTE FUNCTION delete_skill_user_acl_on_org_member_delete();
