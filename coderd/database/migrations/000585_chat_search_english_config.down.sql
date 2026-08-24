-- Revert chat full-text search to the 'simple' text search config.

DROP INDEX idx_chats_title_fts;

CREATE INDEX idx_chats_title_fts ON chats USING GIN (to_tsvector('simple', title));

COMMENT ON INDEX idx_chats_title_fts IS 'Used for full text search. Defined over all rows of the chats table.';

DROP INDEX idx_chat_diff_statuses_pr_title_fts;

CREATE INDEX idx_chat_diff_statuses_pr_title_fts ON chat_diff_statuses USING GIN (to_tsvector('simple', pull_request_title));

COMMENT ON INDEX idx_chat_diff_statuses_pr_title_fts IS 'Used for full text search. Defined over all rows of the chat_diff_statuses table.';

-- Dropped before the reset below so the UPDATE does not maintain an
-- index that is about to be recreated with the old predicate anyway.
DROP INDEX idx_chat_messages_search_tsv_pending;

-- Hand rows re-vectorized with 'english' back to the parent version's
-- bounded dbpurge sweep. The parent backfill only selects rows with
-- search_tsv IS NULL, so 'english' vectors must be reset to NULL to
-- re-enter its pending queue; the parent sweep then rewrites them with
-- 'simple' incrementally, newest first. Rows never re-vectorized
-- (search_tsv_config IS NULL) still hold valid 'simple' lexemes and
-- are left untouched. The reset size is proportional to how much of
-- the backlog the upgraded sweep processed before the rollback, so a
-- prompt rollback rewrites a small set; only a rollback after a full
-- drain approaches a full-table update, which is unavoidable because
-- the parent job has no other way to find these rows.
--
-- This UPDATE runs while the replacement trigger functions (which
-- exclude search_tsv and search_tsv_config from change comparisons)
-- are still installed, so it does not advance message revisions or
-- chat history_version. The original trigger functions are restored
-- below, after this statement.
UPDATE chat_messages SET search_tsv = NULL WHERE search_tsv_config = 'english';

ALTER TABLE chat_messages DROP COLUMN search_tsv_config;

CREATE INDEX idx_chat_messages_search_tsv_pending ON chat_messages USING btree (id DESC)
WHERE ((search_tsv IS NULL) AND (deleted = false) AND (visibility = ANY (ARRAY['user'::chat_message_visibility, 'both'::chat_message_visibility])) AND (role = ANY (ARRAY['user'::chat_message_role, 'assistant'::chat_message_role])));

COMMENT ON INDEX idx_chat_messages_search_tsv_pending IS 'Partial index over chat_messages used for populating search_tsv in the background. Only defined over ''searchable'' rows of chat_messages where search_tsv is NULL.';

-- Restore the original trigger function bodies from 000545.

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

COMMENT ON FUNCTION set_chat_message_revision_before IS 'Component of chatd. Updates chat_snapshot_version when any fields of chat_messages change. Excludes changes to search_tsv as it is not relevant to chatd''s processing loop.';

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
        WHERE (to_jsonb(o) - 'search_tsv') IS DISTINCT FROM (to_jsonb(n) - 'search_tsv')
    ) AS affected
    WHERE c.id = affected.chat_id
      AND (
          c.history_version IS DISTINCT FROM c.snapshot_version
          OR c.generation_attempt <> 0
      );
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION update_chat_history_after_message_update IS 'Component of chatd. Updates history_version and generation_attempt on chats when chat_messages is updated. Excludes changes to search_tsv.';
