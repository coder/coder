DO $$
DECLARE
	previous_lock_timeout text := current_setting('lock_timeout');
	deadline timestamptz := clock_timestamp() + interval '2 minutes';
BEGIN
	LOOP
		BEGIN
			PERFORM set_config('lock_timeout', '100ms', true);
			LOCK TABLE chat_automations IN ACCESS EXCLUSIVE MODE;
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

DROP TRIGGER trigger_enforce_chat_automation_chat_organization ON chat_automations;

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
	RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_enforce_chat_automation_chat_organization
    BEFORE INSERT OR UPDATE OF organization_id, target_chat_id, created_by_chat_id ON chat_automations
    FOR EACH ROW EXECUTE FUNCTION enforce_chat_automation_chat_organization();

DROP INDEX IF EXISTS chat_automations_project_id_idx;

ALTER TABLE chat_automations
    DROP CONSTRAINT chat_automations_project_target_mode,
    DROP COLUMN project_id;
