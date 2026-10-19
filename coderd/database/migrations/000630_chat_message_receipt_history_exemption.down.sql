-- Restore the insert trigger function from before receipts were exempt.
CREATE OR REPLACE FUNCTION update_chat_history_after_message_insert()
RETURNS trigger AS $$
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
$$ LANGUAGE plpgsql;

DROP FUNCTION chat_message_is_structured_output_receipt(chat_message_role, chat_message_visibility, smallint, jsonb);
