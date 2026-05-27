-- Backfill api_key_id for active chat messages that were created before
-- migration 000508 added the column. Uses the chat owner's most recent
-- valid (non-expired) API key as a best-guess surrogate.
WITH active_chats AS (
    SELECT
        c.id,
        c.owner_id
    FROM chats c
    WHERE c.archived = false
      AND c.status::text IN ('pending', 'running', 'waiting', 'paused', 'requires_action')
),
latest_valid_api_key AS (
    SELECT DISTINCT ON (ak.user_id)
        ak.user_id,
        ak.id
    FROM api_keys ak
    WHERE ak.expires_at > now()
    ORDER BY ak.user_id, ak.last_used DESC NULLS LAST
),
active_chats_with_api_key AS (
    SELECT
        ac.id AS chat_id,
        lvak.id AS api_key_id
    FROM active_chats ac
    JOIN latest_valid_api_key lvak ON lvak.user_id = ac.owner_id
)
UPDATE chat_messages cm
SET api_key_id = acwak.api_key_id
FROM active_chats_with_api_key acwak
WHERE cm.chat_id = acwak.chat_id
  AND cm.api_key_id IS NULL;

WITH active_chats AS (
    SELECT
        c.id,
        c.owner_id
    FROM chats c
    WHERE c.archived = false
      AND c.status::text IN ('pending', 'running', 'waiting', 'paused', 'requires_action')
),
latest_valid_api_key AS (
    SELECT DISTINCT ON (ak.user_id)
        ak.user_id,
        ak.id
    FROM api_keys ak
    WHERE ak.expires_at > now()
    ORDER BY ak.user_id, ak.last_used DESC NULLS LAST
),
active_chats_with_api_key AS (
    SELECT
        ac.id AS chat_id,
        lvak.id AS api_key_id
    FROM active_chats ac
    JOIN latest_valid_api_key lvak ON lvak.user_id = ac.owner_id
)
UPDATE chat_queued_messages cqm
SET api_key_id = acwak.api_key_id
FROM active_chats_with_api_key acwak
WHERE cqm.chat_id = acwak.chat_id
  AND cqm.api_key_id IS NULL;
