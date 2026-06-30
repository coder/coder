-- cost_source attributes spend to a feature: NULL is ordinary turn spend,
-- 'summary' and 'title' tag the hidden accounting rows. The CHECK bounds it to
-- that closed set so a typo cannot silently corrupt cost attribution.
ALTER TABLE chat_messages
    ADD COLUMN cost_source TEXT CHECK (cost_source IN ('summary', 'title'));

-- Recreate the AFTER STATEMENT history triggers so only rows with cost_source
-- IS NULL (ordinary turn history) advance history_version. Hidden accounting
-- rows (cost_source set) must not, or the accounting row recorded for a summary
-- would invalidate that same summary's history_version-guarded write.
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
