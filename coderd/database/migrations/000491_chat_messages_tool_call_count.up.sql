-- Adds a precomputed tool_call_count to chat_messages so the
-- /api/experimental/chats/{chat}/turns endpoint can aggregate tool
-- counts per turn without scanning JSONB content per row.
--
-- The counter is denormalized and maintained by application code
-- (InsertChatMessages / UpdateChatMessageByID). The backfill below
-- handles existing rows.
ALTER TABLE chat_messages
    ADD COLUMN tool_call_count smallint NOT NULL DEFAULT 0;

-- Backfill assistant-role rows. Only assistant messages can contain
-- tool-call parts; user, tool, and system roles never do, so we skip
-- them to keep the backfill cheap on large chat_messages tables.
UPDATE chat_messages
SET tool_call_count = LEAST(
    -- Cap at smallint range. A turn with more than 32k tool calls in a
    -- single message would be an outlier we are comfortable saturating.
    32767,
    COALESCE((
        SELECT COUNT(*)
        FROM jsonb_array_elements(content) AS part
        WHERE part->>'type' = 'tool-call'
    ), 0)
)
WHERE role = 'assistant'
    AND content IS NOT NULL
    AND jsonb_typeof(content) = 'array';

-- Partial index over user-role anchor messages within a chat.
-- The /turns endpoint uses this to seek to a paginated window of
-- user messages in O(log N + limit) instead of scanning every
-- visible message in the chat. We include the id column so the
-- index is ORDER BY-friendly for DESC pagination.
CREATE INDEX idx_chat_messages_chat_user_anchor
    ON chat_messages (chat_id, id)
    WHERE role = 'user'
        AND deleted = false
        AND visibility IN ('user', 'both');
