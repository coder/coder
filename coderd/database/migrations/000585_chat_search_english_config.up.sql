-- Switch chat message full-text search from the 'simple' to the
-- 'english' text search config so queries match inflected forms
-- (e.g. "refactor" matches "refactoring"). No data or index changes
-- here: rewriting search_tsv in a migration would block message
-- writes on large tables. The new search_tsv_config column records
-- which config produced each stored vector, and the bounded dbpurge
-- sweep rewrites stale rows incrementally, newest first (see
-- ReindexStaleChatMessagesSearchTsv).

-- The Postgres text search configs this system has produced vectors
-- with. Extend with ALTER TYPE ... ADD VALUE if the config changes
-- again.
CREATE TYPE chat_message_search_tsv_config AS ENUM ('simple', 'english');

ALTER TABLE chat_messages ADD COLUMN search_tsv_config chat_message_search_tsv_config;

COMMENT ON COLUMN chat_messages.search_tsv_config IS 'Text search config that produced search_tsv. NULL means an unknown config (a pre-migration vector or one written by an old binary); the dbpurge sweep re-vectorizes such rows.';

-- Both chatd trigger functions must exclude search_tsv_config
-- (alongside search_tsv) from their change comparisons so backfill
-- batches do not advance message revisions or chat history_version.

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

    NEW.revision = chat_snapshot_version;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION set_chat_message_revision_before IS 'Component of chatd. Updates chat_snapshot_version when any fields of chat_messages change. Excludes changes to search_tsv and search_tsv_config as they are not relevant to chatd''s processing loop.';

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
        WHERE (to_jsonb(o) - 'search_tsv' - 'search_tsv_config') IS DISTINCT FROM (to_jsonb(n) - 'search_tsv' - 'search_tsv_config')
    ) AS affected
    WHERE c.id = affected.chat_id
      AND (
          c.history_version IS DISTINCT FROM c.snapshot_version
          OR c.generation_attempt <> 0
      );
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION update_chat_history_after_message_update IS 'Component of chatd. Updates history_version and generation_attempt on chats when chat_messages is updated. Excludes changes to search_tsv and search_tsv_config.';
