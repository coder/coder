CREATE FUNCTION chat_message_search_text(content jsonb) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT CASE WHEN jsonb_typeof(content) = 'array' THEN (
        SELECT string_agg(part->>'text', ' ' ORDER BY ordinality)
        FROM jsonb_array_elements(content) WITH ORDINALITY AS t(part, ordinality)
        WHERE part->>'type' = 'text'
    ) END
$$;

COMMENT ON FUNCTION chat_message_search_text IS 'Extracts searchable content from chat_messages. Returns NULL for scalar JSON strings (content_version=0). Immutable as it is used in indexes.';

-- Populated by a background sweep, not at insert time. NULL means pending.
ALTER TABLE chat_messages ADD COLUMN search_tsv tsvector;

COMMENT ON COLUMN chat_messages.search_tsv IS 'Used for full text search. NULL initially, populated async via background job.';

CREATE INDEX idx_chat_messages_search_tsv ON chat_messages
USING GIN (search_tsv)
WHERE ((search_tsv IS NOT NULL) AND (deleted = false) AND (visibility = ANY (ARRAY['user'::chat_message_visibility, 'both'::chat_message_visibility])) AND (role = ANY (ARRAY['user'::chat_message_role, 'assistant'::chat_message_role])));

COMMENT ON INDEX idx_chat_messages_search_tsv IS 'Partial index over chat_messages used for full text search. Only defined over ''searchable'' rows of chat_messages.';

CREATE INDEX idx_chat_messages_search_tsv_pending ON chat_messages USING btree (id DESC)
WHERE ((search_tsv IS NULL) AND (deleted = false) AND (visibility = ANY (ARRAY['user'::chat_message_visibility, 'both'::chat_message_visibility])) AND (role = ANY (ARRAY['user'::chat_message_role, 'assistant'::chat_message_role])));

COMMENT ON INDEX idx_chat_messages_search_tsv IS 'Partial index over chat_messages used for populating search_tsv in the background. Only defined over ''searchable'' rows of chat_messages where search_tsv is NULL.';

CREATE INDEX idx_chats_title_fts ON chats USING GIN (to_tsvector('simple', title));

COMMENT ON index idx_chats_title_fts IS 'Used for full text search. Defined over all rows of the chats table.';

CREATE INDEX idx_chat_diff_statuses_pr_title_fts ON chat_diff_statuses USING GIN (to_tsvector('simple', pull_request_title));

COMMENT ON index idx_chats_title_fts IS 'Used for full text search. Defined over all rows of the chats table.';

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
