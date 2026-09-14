-- Partial index matching the AutoArchiveInactiveChats WHERE clause so
-- dbpurge can skip the bulk of archived / pinned / child chats.
-- The status predicate lives in the query, not the index, because
-- enum values added by earlier migrations cannot be referenced in
-- index predicates within the same transaction batch.
CREATE INDEX IF NOT EXISTS idx_chats_auto_archive_candidates
    ON chats (created_at)
    WHERE archived = false
      AND pin_order = 0
      AND parent_chat_id IS NULL;
