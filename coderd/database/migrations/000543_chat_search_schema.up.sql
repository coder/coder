-- CASE guard: jsonb_array_elements raises on non-array input. Legacy
-- content_version=0 rows store scalar JSON strings; excluded from search
-- by design. IMMUTABLE for expression index use.
CREATE FUNCTION chat_message_search_text(content jsonb) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT CASE WHEN jsonb_typeof(content) = 'array' THEN (
        SELECT string_agg(part->>'text', ' ' ORDER BY ordinality)
        FROM jsonb_array_elements(content) WITH ORDINALITY AS t(part, ordinality)
        WHERE part->>'type' = 'text'
    ) END
$$;

-- Populated by a background sweep, not at insert time. NULL means pending.
ALTER TABLE chat_messages ADD COLUMN search_tsv tsvector;

CREATE INDEX idx_chat_messages_search_tsv ON chat_messages
USING GIN (search_tsv)
WHERE ((search_tsv IS NOT NULL) AND (deleted = false) AND (visibility = ANY (ARRAY['user'::chat_message_visibility, 'both'::chat_message_visibility])) AND (role = ANY (ARRAY['user'::chat_message_role, 'assistant'::chat_message_role])));

-- Sweep uses this to find pending rows. Queries must repeat the full predicate.
CREATE INDEX idx_chat_messages_search_tsv_pending ON chat_messages USING btree (id DESC)
WHERE ((search_tsv IS NULL) AND (deleted = false) AND (visibility = ANY (ARRAY['user'::chat_message_visibility, 'both'::chat_message_visibility])) AND (role = ANY (ARRAY['user'::chat_message_role, 'assistant'::chat_message_role])));

CREATE INDEX idx_chats_title_fts ON chats
    USING GIN (to_tsvector('simple', title));

CREATE INDEX idx_chat_diff_statuses_pr_title_fts ON chat_diff_statuses
    USING GIN (to_tsvector('simple', pull_request_title));

-- search_tsv is system-maintained; the backfill must not perturb message
-- revision, chat history_version, generation_attempt, or retry state.
-- Both triggers therefore ignore changes confined to search_tsv.
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
        -- jsonb-minus here: transition-table rows have no composite-copy idiom in pure SQL.
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
