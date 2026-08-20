-- Switch chat full-text search from the 'simple' to the 'english' text
-- search config so queries match inflected forms (e.g. searching
-- "refactor" matches "refactoring"). The config baked into a tsvector
-- must match the config used by the tsquery at search time.
--
-- The title expression indexes are rebuilt here; chats and
-- chat_diff_statuses hold one row per chat, so the rebuild is cheap.
-- The stored chat_messages.search_tsv vectors are NOT rewritten here:
-- chat_messages can be large, and a full-table UPDATE inside the
-- migration transaction would rewrite every row, churn both partial
-- indexes, and hold the locks and WAL until the whole migration batch
-- commits. Instead the new search_tsv_config column records which
-- config produced each stored vector, the pending-queue index treats
-- any row whose config is not 'english' as pending, and the bounded
-- dbpurge sweep rewrites stale rows incrementally, newest first.

DROP INDEX idx_chats_title_fts;

CREATE INDEX idx_chats_title_fts ON chats USING GIN (to_tsvector('english', title));

COMMENT ON INDEX idx_chats_title_fts IS 'Used for full text search. Defined over all rows of the chats table.';

DROP INDEX idx_chat_diff_statuses_pr_title_fts;

CREATE INDEX idx_chat_diff_statuses_pr_title_fts ON chat_diff_statuses USING GIN (to_tsvector('english', pull_request_title));

COMMENT ON INDEX idx_chat_diff_statuses_pr_title_fts IS 'Used for full text search. Defined over all rows of the chat_diff_statuses table.';

ALTER TABLE chat_messages ADD COLUMN search_tsv_config text;

COMMENT ON COLUMN chat_messages.search_tsv_config IS 'Text search config that produced search_tsv. NULL means the vector is stale (produced by an unknown config) and the row is pending re-vectorization. Binaries that predate this column cannot set it, so vectors written by an old replica during a rolling upgrade stay pending and are rewritten by an upgraded replica''s sweep.';

-- A row is pending when it has never been vectorized (search_tsv IS
-- NULL) or when its stored vector was produced with a different config
-- (search_tsv_config IS DISTINCT FROM 'english'). All pre-existing
-- rows have search_tsv_config NULL, so the entire eligible backlog
-- re-enters this queue with no data movement; the index shrinks as the
-- sweep drains it.
DROP INDEX idx_chat_messages_search_tsv_pending;

CREATE INDEX idx_chat_messages_search_tsv_pending ON chat_messages USING btree (id DESC)
WHERE (((search_tsv IS NULL) OR (search_tsv_config IS DISTINCT FROM 'english')) AND (deleted = false) AND (visibility = ANY (ARRAY['user'::chat_message_visibility, 'both'::chat_message_visibility])) AND (role = ANY (ARRAY['user'::chat_message_role, 'assistant'::chat_message_role])));

COMMENT ON INDEX idx_chat_messages_search_tsv_pending IS 'Partial index over chat_messages used for populating search_tsv in the background. Only defined over ''searchable'' rows of chat_messages whose search_tsv is NULL or was produced with a stale text search config.';

-- The sweep now also writes search_tsv_config, so both chatd trigger
-- functions must exclude it (alongside search_tsv) when deciding
-- whether a message change is meaningful. Without this every backfill
-- batch would advance message revisions and chat history_version,
-- waking chatd's processing loop for the whole backlog.

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
