-- cost_source attributes spend on a chat_message to a specific feature so that
-- summary and title generation spend can be reported separately from ordinary
-- turn spend. NULL means ordinary turn spend; 'summary' and 'title' tag the
-- hidden accounting rows written for background summary and manual title
-- generation respectively. The CHECK constrains it to the closed discriminator
-- set so a typo or direct SQL write cannot silently corrupt cost attribution
-- (NULL is implicitly allowed).
ALTER TABLE chat_messages
    ADD COLUMN cost_source TEXT CHECK (cost_source IN ('summary', 'title'));

-- Background summary and manual title generation write hidden, soft-deleted
-- chat_messages accounting rows tagged with cost_source to attribute their
-- spend. Those rows are not durable conversation history, so they must not
-- advance chats.history_version. Otherwise a summary write guarded on
-- history_version is invalidated by the very accounting row recorded for that
-- same summary, and the write (and the last_turn_summary write racing behind
-- it) is rejected as stale even when no new turn occurred.
--
-- Recreate the AFTER STATEMENT history triggers so only rows with
-- cost_source IS NULL (ordinary turn history) advance history_version.
-- snapshot_version still advances for every change, so history_version
-- correctly trails behind it after an accounting-only write and catches up on
-- the next real message.
CREATE OR REPLACE FUNCTION update_chat_history_after_message_insert()
RETURNS trigger AS $$
BEGIN
    UPDATE chats c
    SET history_version = c.snapshot_version,
        generation_attempt = 0
    FROM (
        SELECT DISTINCT chat_id
        FROM chat_message_history_new_rows
        WHERE cost_source IS NULL
    ) AS affected
    WHERE c.id = affected.chat_id
      AND (
          c.history_version IS DISTINCT FROM c.snapshot_version
          OR c.generation_attempt <> 0
      );
    RETURN NULL;
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
          AND n.cost_source IS NULL
    ) AS affected
    WHERE c.id = affected.chat_id
      AND (
          c.history_version IS DISTINCT FROM c.snapshot_version
          OR c.generation_attempt <> 0
      );
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
