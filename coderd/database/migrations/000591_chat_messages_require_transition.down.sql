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

COMMENT ON FUNCTION update_chat_history_after_message_insert() IS NULL;

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

COMMENT ON FUNCTION update_chat_history_after_message_update() IS 'Component of chatd. Updates history_version and generation_attempt on chats when chat_messages is updated. Excludes changes to search_tsv and search_tsv_config.';
