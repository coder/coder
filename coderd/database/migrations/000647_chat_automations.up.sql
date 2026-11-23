CREATE TYPE chat_automation_kind AS ENUM ('schedule');

CREATE TYPE chat_automation_target_mode AS ENUM ('existing_chat', 'new_chat');

CREATE TYPE chat_automation_run_outcome AS ENUM ('accepted', 'skipped', 'refused');

CREATE TABLE chat_automations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    owner_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 128),
    created_by_chat_id uuid REFERENCES chats (id) ON DELETE SET NULL,
    kind chat_automation_kind NOT NULL,
    enabled boolean NOT NULL DEFAULT false,
    prompt text NOT NULL,
    target_mode chat_automation_target_mode NOT NULL,
    target_chat_id uuid REFERENCES chats (id) ON DELETE SET NULL,
    new_chat_model_config_id uuid,
    project_id uuid REFERENCES chat_projects (id) ON DELETE SET NULL,
    schedule_cron text,
    schedule_time_zone text,
    schedule_next_run_at timestamptz,
    last_run_at timestamptz,
    last_run_outcome chat_automation_run_outcome,
    last_run_reason text,
    last_run_chat_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- Keeps the new-chat model in the automation's organization, matching
    -- the composite foreign keys on chat_*_model_overrides.
    CONSTRAINT chat_automations_new_chat_model_config_fkey
        FOREIGN KEY (organization_id, new_chat_model_config_id)
        REFERENCES chat_model_configs (organization_id, id),
    CONSTRAINT chat_automations_schedule_shape
        CHECK ((kind = 'schedule') = (schedule_cron IS NOT NULL AND schedule_time_zone IS NOT NULL)),
    -- target_chat_id is not required for existing_chat: deleting the target
    -- chat sets it to NULL.
    CONSTRAINT chat_automations_target_shape CHECK (
        (target_mode = 'existing_chat' AND new_chat_model_config_id IS NULL)
        OR (target_mode = 'new_chat' AND target_chat_id IS NULL AND new_chat_model_config_id IS NOT NULL)
    ),
    CONSTRAINT chat_automations_project_target_mode
        CHECK (project_id IS NULL OR target_mode = 'new_chat'),
    CONSTRAINT chat_automations_last_run_shape
        CHECK ((last_run_at IS NULL) = (last_run_outcome IS NULL))
);

COMMENT ON TABLE chat_automations IS 'Saved prompts that a trigger sends to a chat as their owner.';
COMMENT ON COLUMN chat_automations.created_by_chat_id IS 'Chat whose agent created the automation. NULL when a person created it or after that chat is deleted.';
COMMENT ON COLUMN chat_automations.target_chat_id IS 'Target of an existing_chat automation. NULL after the target chat is deleted.';
COMMENT ON COLUMN chat_automations.project_id IS 'Project for the chats a new_chat automation creates. NULL when it has none or after the project is deleted.';
COMMENT ON COLUMN chat_automations.schedule_next_run_at IS 'The next occurrence to run. NULL when none is pending.';
COMMENT ON COLUMN chat_automations.last_run_chat_id IS 'Chat of the latest run. No foreign key: the chat can be deleted.';

CREATE INDEX chat_automations_org_owner_idx ON chat_automations (organization_id, owner_id);
-- Serves the per-owner count across organizations and the cleanup when a
-- user is deleted.
CREATE INDEX chat_automations_owner_id_idx ON chat_automations (owner_id);
CREATE INDEX chat_automations_due_idx ON chat_automations (schedule_next_run_at) WHERE kind = 'schedule' AND enabled;
-- Chat and project deletion enforce ON DELETE SET NULL through these columns.
CREATE INDEX chat_automations_target_chat_id_idx ON chat_automations (target_chat_id) WHERE target_chat_id IS NOT NULL;
CREATE INDEX chat_automations_created_by_chat_id_idx ON chat_automations (created_by_chat_id) WHERE created_by_chat_id IS NOT NULL;
CREATE INDEX chat_automations_project_id_idx ON chat_automations (project_id) WHERE project_id IS NOT NULL;

CREATE FUNCTION insert_chat_automation_fail_if_user_deleted() RETURNS trigger
    LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM 1
    FROM users
    WHERE id = NEW.owner_id
      AND deleted = true
    LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION 'Cannot create chat_automation for deleted user'
            USING ERRCODE = 'check_violation',
                  CONSTRAINT = 'chat_automation_owner_deleted';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_insert_chat_automations
    BEFORE INSERT ON chat_automations
    FOR EACH ROW
    EXECUTE FUNCTION insert_chat_automation_fail_if_user_deleted();

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

        -- Remove their chat automations, which would otherwise keep
        -- running as them.
        -- chat_automations.owner_id has ON DELETE CASCADE, but soft-delete
        -- does not remove the users row so the FK cascade never fires.
        DELETE FROM chat_automations
        WHERE owner_id = OLD.id;
    END IF;
    RETURN NEW;
END;
$$;

-- 000616 removed these values with the earlier automations experiment.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:delete';
