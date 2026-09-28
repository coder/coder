-- Restore the trigger-maintained versions: messages are stamped with the
-- current snapshot_version (bumped at the start of the transaction) and the
-- history and queue versions are written by triggers.

CREATE OR REPLACE FUNCTION set_chat_message_revision_before() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    chat_snapshot_version bigint;
    cmp chat_messages;
BEGIN
    IF TG_OP = 'INSERT' AND NEW.revision IS NOT NULL THEN
        RAISE EXCEPTION 'chat_messages.revision must be assigned by trigger';
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF OLD.chat_id IS DISTINCT FROM NEW.chat_id THEN
            RAISE EXCEPTION 'chat_messages.chat_id is immutable';
        END IF;

        IF OLD.revision IS DISTINCT FROM NEW.revision THEN
            RAISE EXCEPTION 'chat_messages.revision must be assigned by trigger';
        END IF;

        cmp := NEW;
        cmp.search_tsv := OLD.search_tsv;
        cmp.search_tsv_config := OLD.search_tsv_config;
        IF OLD IS NOT DISTINCT FROM cmp THEN
            RETURN NEW;
        END IF;
    END IF;

    SELECT snapshot_version INTO chat_snapshot_version
    FROM chats WHERE id = NEW.chat_id;

    IF chat_snapshot_version IS NULL THEN
        RAISE EXCEPTION 'chat % does not exist', NEW.chat_id;
    END IF;

    NEW.revision = chat_snapshot_version;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION update_chat_history_after_message_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    UPDATE chats c
    SET history_version = c.snapshot_version,
        generation_attempt = 0
    FROM (
        SELECT DISTINCT chat_id FROM chat_message_history_new_rows
    ) AS affected
    WHERE c.id = affected.chat_id
      AND (
          c.history_version IS DISTINCT FROM c.snapshot_version
          OR c.generation_attempt <> 0
      );
    RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION update_chat_history_after_message_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    UPDATE chats c
    SET history_version = c.snapshot_version,
        generation_attempt = 0
    FROM (
        SELECT DISTINCT n.chat_id
        FROM chat_message_history_new_rows n
        JOIN chat_message_history_old_rows o ON o.id = n.id
        WHERE (to_jsonb(o) - 'search_tsv' - 'search_tsv_config') IS DISTINCT FROM (to_jsonb(n) - 'search_tsv' - 'search_tsv_config')
    ) AS affected
    WHERE c.id = affected.chat_id
      AND (
          c.history_version IS DISTINCT FROM c.snapshot_version
          OR c.generation_attempt <> 0
      );
    RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION bump_chat_queue_version_on_queued_message_change() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    changed_chat_id uuid;
BEGIN
    IF TG_OP = 'DELETE' THEN
        changed_chat_id = OLD.chat_id;
    ELSE
        changed_chat_id = NEW.chat_id;
    END IF;

    UPDATE chats
    SET queue_version = snapshot_version
    WHERE id = changed_chat_id;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_bump_chat_queue_version_on_queued_message_delete AFTER DELETE ON chat_queued_messages FOR EACH ROW EXECUTE FUNCTION bump_chat_queue_version_on_queued_message_change();
CREATE TRIGGER trigger_bump_chat_queue_version_on_queued_message_insert AFTER INSERT ON chat_queued_messages FOR EACH ROW EXECUTE FUNCTION bump_chat_queue_version_on_queued_message_change();
CREATE TRIGGER trigger_bump_chat_queue_version_on_queued_message_update AFTER UPDATE OF content, model_config_id, "position", created_by ON chat_queued_messages FOR EACH ROW EXECUTE FUNCTION bump_chat_queue_version_on_queued_message_change();
CREATE TRIGGER trigger_update_chat_history_after_message_insert AFTER INSERT ON chat_messages REFERENCING NEW TABLE AS chat_message_history_new_rows FOR EACH STATEMENT EXECUTE FUNCTION update_chat_history_after_message_insert();
CREATE TRIGGER trigger_update_chat_history_after_message_update AFTER UPDATE ON chat_messages REFERENCING OLD TABLE AS chat_message_history_old_rows NEW TABLE AS chat_message_history_new_rows FOR EACH STATEMENT EXECUTE FUNCTION update_chat_history_after_message_update();
