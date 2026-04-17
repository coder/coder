-- Partial index supporting the AutoArchiveInactiveChats CTE predicate.
-- Auto-archive only considers active (archived = false), unpinned
-- (pin_order = 0) root chats (parent_chat_id IS NULL), so a partial
-- index lets dbpurge jump straight to candidates without scanning the
-- full chats table even in deployments with millions of archived or
-- cascaded chats.
CREATE INDEX IF NOT EXISTS idx_chats_auto_archive_candidates
    ON chats (created_at)
    WHERE archived = false
      AND pin_order = 0
      AND parent_chat_id IS NULL;
