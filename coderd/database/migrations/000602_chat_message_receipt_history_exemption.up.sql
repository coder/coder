-- A structured output receipt records how a request ended. It can be written
-- while an unrelated turn is running, so inserting it must not reset that
-- turn's execution fence (history_version and generation_attempt). Its
-- revision is still assigned from snapshot_version, so streams deliver it.
--
-- chat_message_is_structured_output_receipt matches only the exact receipt
-- shape: an assistant row visible only to the user, content version 1, and a
-- content array holding exactly one structured-output-outcome part with every
-- other part a text part. A discriminator nested anywhere else does not count.
CREATE FUNCTION chat_message_is_structured_output_receipt(
    message_role chat_message_role,
    message_visibility chat_message_visibility,
    message_content_version smallint,
    message_content jsonb
) RETURNS boolean
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT CASE
        WHEN message_role = 'assistant'
            AND message_visibility = 'user'
            AND message_content_version = 1
            AND jsonb_typeof(message_content) = 'array'
        THEN (
            SELECT count(*) FILTER (WHERE part->>'type' = 'structured-output-outcome') = 1
                AND bool_and(COALESCE(part->>'type' IN ('structured-output-outcome', 'text'), false))
            FROM jsonb_array_elements(message_content) AS part
        )
        ELSE false
    END;
$$;

-- Receipts are filtered out per row before affected chats are derived, so a
-- statement that also inserts an ordinary row for the same chat still resets
-- its execution history.
CREATE OR REPLACE FUNCTION update_chat_history_after_message_insert()
RETURNS trigger AS $$
BEGIN
    UPDATE chats c
    SET history_version = c.snapshot_version,
        generation_attempt = 0
    FROM (
        SELECT DISTINCT n.chat_id
        FROM chat_message_history_new_rows n
        WHERE NOT chat_message_is_structured_output_receipt(n.role, n.visibility, n.content_version, n.content)
    ) AS affected
    WHERE c.id = affected.chat_id
      AND (
          c.history_version IS DISTINCT FROM c.snapshot_version
          OR c.generation_attempt <> 0
      );
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
