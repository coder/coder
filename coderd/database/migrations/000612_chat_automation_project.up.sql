-- Take the locks this migration needs before changing anything, retrying
-- short waits so that concurrent chatd traffic cannot deadlock with it. See
-- 000609_chat_automations.up.sql for the reasoning.
DO $$
DECLARE
	previous_lock_timeout text := current_setting('lock_timeout');
	deadline timestamptz := clock_timestamp() + interval '2 minutes';
BEGIN
	LOOP
		BEGIN
			PERFORM set_config('lock_timeout', '100ms', true);
			LOCK TABLE chat_automations IN ACCESS EXCLUSIVE MODE;
			-- The chat_automations.project_id foreign key needs this.
			LOCK TABLE chat_projects IN SHARE ROW EXCLUSIVE MODE;
			EXIT;
		EXCEPTION WHEN lock_not_available OR deadlock_detected THEN
			IF clock_timestamp() > deadline THEN
				RAISE EXCEPTION 'migration 000612 could not lock the chat automation tables within 2 minutes';
			END IF;
		END;
		PERFORM pg_sleep(0.1);
	END LOOP;
	PERFORM set_config('lock_timeout', previous_lock_timeout, true);
END;
$$;

ALTER TABLE chat_automations
    ADD COLUMN project_id uuid REFERENCES chat_projects (id) ON DELETE SET NULL,
    ADD CONSTRAINT chat_automations_project_target_mode CHECK (project_id IS NULL OR target_mode = 'new_chat');

COMMENT ON COLUMN chat_automations.project_id IS 'Project for chats a new_chat automation creates. NULL when the automation has no project or after the project is deleted.';

-- Project deletion enforces ON DELETE SET NULL through this column.
CREATE INDEX chat_automations_project_id_idx ON chat_automations (project_id) WHERE project_id IS NOT NULL;

-- A composite foreign key cannot keep these references in the automation's
-- organization: ON DELETE SET NULL would also clear organization_id, and
-- column-list SET NULL needs PostgreSQL 15.
CREATE OR REPLACE FUNCTION enforce_chat_automation_chat_organization() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	-- Validate a reference only when it or organization_id changes. Chat
	-- deletion runs one ON DELETE SET NULL update per reference, and the
	-- other, unchanged reference may point at a chat the same statement is
	-- deleting.
	IF NEW.target_chat_id IS NOT NULL AND (
		TG_OP = 'INSERT'
		OR NEW.target_chat_id IS DISTINCT FROM OLD.target_chat_id
		OR NEW.organization_id IS DISTINCT FROM OLD.organization_id
	) AND NOT EXISTS (
		SELECT 1 FROM chats
		WHERE id = NEW.target_chat_id AND organization_id = NEW.organization_id
	) THEN
		RAISE EXCEPTION 'target chat % is not in organization %', NEW.target_chat_id, NEW.organization_id
			USING ERRCODE = 'check_violation',
			      CONSTRAINT = 'chat_automations_chat_organization';
	END IF;
	IF NEW.created_by_chat_id IS NOT NULL AND (
		TG_OP = 'INSERT'
		OR NEW.created_by_chat_id IS DISTINCT FROM OLD.created_by_chat_id
		OR NEW.organization_id IS DISTINCT FROM OLD.organization_id
	) AND NOT EXISTS (
		SELECT 1 FROM chats
		WHERE id = NEW.created_by_chat_id AND organization_id = NEW.organization_id
	) THEN
		RAISE EXCEPTION 'creating chat % is not in organization %', NEW.created_by_chat_id, NEW.organization_id
			USING ERRCODE = 'check_violation',
			      CONSTRAINT = 'chat_automations_chat_organization';
	END IF;
	-- Project deletion sets project_id to NULL, which skips this check.
	IF NEW.project_id IS NOT NULL AND (
		TG_OP = 'INSERT'
		OR NEW.project_id IS DISTINCT FROM OLD.project_id
		OR NEW.organization_id IS DISTINCT FROM OLD.organization_id
	) AND NOT EXISTS (
		SELECT 1 FROM chat_projects
		WHERE id = NEW.project_id AND organization_id = NEW.organization_id
	) THEN
		RAISE EXCEPTION 'project % is not in organization %', NEW.project_id, NEW.organization_id
			USING ERRCODE = 'check_violation',
			      CONSTRAINT = 'chat_automations_project_organization';
	END IF;
	RETURN NEW;
END;
$$;

DROP TRIGGER trigger_enforce_chat_automation_chat_organization ON chat_automations;
CREATE TRIGGER trigger_enforce_chat_automation_chat_organization
    BEFORE INSERT OR UPDATE OF organization_id, target_chat_id, created_by_chat_id, project_id ON chat_automations
    FOR EACH ROW EXECUTE FUNCTION enforce_chat_automation_chat_organization();
