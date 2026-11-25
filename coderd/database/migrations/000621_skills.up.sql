-- Generalize personal skills into one skills table owned by a user, an
-- organization, or a project.
ALTER TABLE user_skills RENAME TO skills;

ALTER TABLE skills RENAME CONSTRAINT user_skills_pkey TO skills_pkey;
ALTER TABLE skills RENAME CONSTRAINT user_skills_user_id_fkey TO skills_user_id_fkey;
ALTER TABLE skills RENAME CONSTRAINT user_skills_name_size TO skills_name_size;
ALTER TABLE skills RENAME CONSTRAINT user_skills_name_format TO skills_name_format;
ALTER TABLE skills RENAME CONSTRAINT user_skills_description_size TO skills_description_size;
ALTER TABLE skills RENAME CONSTRAINT user_skills_content_size TO skills_content_size;

ALTER INDEX user_skills_user_id_name_idx RENAME TO skills_user_id_name_idx;

-- The per-user cap and the deleted-user guard are replaced below with
-- owner-aware versions.
DROP TRIGGER trigger_user_skills_per_user_limit ON skills;
DROP FUNCTION enforce_user_skills_per_user_limit();
DROP TRIGGER trigger_upsert_user_skills ON skills;

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

CREATE TRIGGER trigger_upsert_skills
    BEFORE INSERT OR UPDATE ON skills
    FOR EACH ROW
    WHEN (NEW.user_id IS NOT NULL)
    EXECUTE FUNCTION insert_user_skill_fail_if_user_deleted();

-- PL/pgSQL bodies are stored as text, so table renames do not reach them.
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

        -- Remove their personal skills.
        -- skills.user_id has ON DELETE CASCADE, but soft-delete
        -- does not remove the users row so the FK cascade never fires.
        DELETE FROM skills
        WHERE user_id = OLD.id;
    END IF;
    RETURN NEW;
END;
$$;

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

ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'organization_skill';

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization_skill:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization_skill:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization_skill:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization_skill:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization_skill:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization_skill:share';
