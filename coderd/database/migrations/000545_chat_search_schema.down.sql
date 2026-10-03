-- Restore the original trigger bodies from 000519.
CREATE OR REPLACE FUNCTION set_chat_message_revision_before()
RETURNS trigger AS $$
DECLARE
    chat_snapshot_version bigint;
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

        IF OLD IS NOT DISTINCT FROM NEW THEN
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
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_chat_history_after_message_update()
RETURNS trigger AS $$
BEGIN
    UPDATE chats c
    SET history_version = c.snapshot_version,
        generation_attempt = 0
    FROM (
        SELECT DISTINCT n.chat_id
        FROM chat_message_history_new_rows n
        JOIN chat_message_history_old_rows o ON o.id = n.id
        WHERE o IS DISTINCT FROM n
    ) AS affected
    WHERE c.id = affected.chat_id
      AND (
          c.history_version IS DISTINCT FROM c.snapshot_version
          OR c.generation_attempt <> 0
      );
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP INDEX IF EXISTS idx_chat_diff_statuses_pr_title_fts;

DROP INDEX IF EXISTS idx_chats_title_fts;

DROP INDEX IF EXISTS idx_chat_messages_search_tsv_pending;

DROP INDEX IF EXISTS idx_chat_messages_search_tsv;

ALTER TABLE chat_messages DROP COLUMN IF EXISTS search_tsv;

DROP FUNCTION IF EXISTS chat_message_search_text(jsonb);
