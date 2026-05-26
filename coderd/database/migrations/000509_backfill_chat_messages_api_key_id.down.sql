-- The down migration for 000508 drops the api_key_id column entirely,
-- so this is effectively a no-op. We NULL out only for correctness if
-- this migration is rolled back in isolation.
UPDATE chat_messages cm
SET api_key_id = NULL
WHERE cm.api_key_id IS NOT NULL
  AND EXISTS (
      SELECT 1 FROM chats c
      WHERE c.id = cm.chat_id
        AND c.archived = false
        AND c.status IN ('pending', 'running', 'waiting')
  );

UPDATE chat_queued_messages cqm
SET api_key_id = NULL
WHERE cqm.api_key_id IS NOT NULL
  AND EXISTS (
      SELECT 1 FROM chats c
      WHERE c.id = cqm.chat_id
        AND c.archived = false
        AND c.status IN ('pending', 'running', 'waiting')
  );
