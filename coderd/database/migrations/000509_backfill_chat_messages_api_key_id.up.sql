-- Backfill api_key_id for active chat messages that were created before
-- migration 000508 added the column. Uses the chat owner's most recent
-- valid (non-expired) API key as a best-guess surrogate.
UPDATE chat_messages cm
SET api_key_id = (
    SELECT ak.id
    FROM api_keys ak
    JOIN chats c ON c.owner_id = ak.user_id
    WHERE c.id = cm.chat_id
      AND ak.expires_at > now()
    ORDER BY ak.last_used DESC NULLS LAST
    LIMIT 1
)
WHERE cm.api_key_id IS NULL
  AND EXISTS (
      SELECT 1 FROM chats c
      WHERE c.id = cm.chat_id
        AND c.archived = false
        AND c.status IN ('pending', 'running', 'waiting')
  );

UPDATE chat_queued_messages cqm
SET api_key_id = (
    SELECT ak.id
    FROM api_keys ak
    JOIN chats c ON c.owner_id = ak.user_id
    WHERE c.id = cqm.chat_id
      AND ak.expires_at > now()
    ORDER BY ak.last_used DESC NULLS LAST
    LIMIT 1
)
WHERE cqm.api_key_id IS NULL
  AND EXISTS (
      SELECT 1 FROM chats c
      WHERE c.id = cqm.chat_id
        AND c.archived = false
        AND c.status IN ('pending', 'running', 'waiting')
  );
