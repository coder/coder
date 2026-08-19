-- Revert chat full-text search to the 'simple' text search config.

DROP INDEX idx_chats_title_fts;

CREATE INDEX idx_chats_title_fts ON chats USING GIN (to_tsvector('simple', title));

COMMENT ON INDEX idx_chats_title_fts IS 'Used for full text search. Defined over all rows of the chats table.';

DROP INDEX idx_chat_diff_statuses_pr_title_fts;

CREATE INDEX idx_chat_diff_statuses_pr_title_fts ON chat_diff_statuses USING GIN (to_tsvector('simple', pull_request_title));

COMMENT ON INDEX idx_chat_diff_statuses_pr_title_fts IS 'Used for full text search. Defined over all rows of the chat_diff_statuses table.';

-- Reset stored vectors so the backfill sweep repopulates them with the
-- 'simple' config.
UPDATE chat_messages SET search_tsv = NULL WHERE search_tsv IS NOT NULL;
