-- A chat transition must update the chats row exactly once. Postgres re-runs
-- every foreign key check on an UPDATE whose old row version was written by
-- the current transaction, and each check takes FOR KEY SHARE on the parent
-- row (users, organizations, chat_model_configs). Those parents are shared by
-- many chats, so the trigger-driven writes below serialized every transition
-- on the same few heap pages. The transition's own commit write now advances
-- snapshot_version, history_version, queue_version, and generation_attempt.

-- Messages are stamped with the version the enclosing transaction commits:
-- the commit write runs after all message writes and increments
-- snapshot_version by one.
CREATE OR REPLACE FUNCTION set_chat_message_revision_before()
RETURNS trigger AS $$
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

    NEW.revision = chat_snapshot_version + 1;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER trigger_update_chat_history_after_message_insert ON chat_messages;
DROP TRIGGER trigger_update_chat_history_after_message_update ON chat_messages;
DROP FUNCTION update_chat_history_after_message_insert();
DROP FUNCTION update_chat_history_after_message_update();

DROP TRIGGER trigger_bump_chat_queue_version_on_queued_message_insert ON chat_queued_messages;
DROP TRIGGER trigger_bump_chat_queue_version_on_queued_message_update ON chat_queued_messages;
DROP TRIGGER trigger_bump_chat_queue_version_on_queued_message_delete ON chat_queued_messages;
DROP FUNCTION bump_chat_queue_version_on_queued_message_change();
