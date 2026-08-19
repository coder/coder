-- Switch chat full-text search from the 'simple' to the 'english' text
-- search config so queries match inflected forms (e.g. searching
-- "refactor" matches "refactoring"). The config baked into a tsvector
-- must match the config used by the tsquery at search time, so the
-- expression indexes and the stored chat_messages.search_tsv column
-- must all be rebuilt with 'english'.

DROP INDEX idx_chats_title_fts;

CREATE INDEX idx_chats_title_fts ON chats USING GIN (to_tsvector('english', title));

COMMENT ON INDEX idx_chats_title_fts IS 'Used for full text search. Defined over all rows of the chats table.';

DROP INDEX idx_chat_diff_statuses_pr_title_fts;

CREATE INDEX idx_chat_diff_statuses_pr_title_fts ON chat_diff_statuses USING GIN (to_tsvector('english', pull_request_title));

COMMENT ON INDEX idx_chat_diff_statuses_pr_title_fts IS 'Used for full text search. Defined over all rows of the chat_diff_statuses table.';

-- Reset stored vectors so the dbpurge backfill sweep repopulates them
-- with the 'english' config. NULL means "pending": eligible rows
-- re-enter idx_chat_messages_search_tsv_pending and are rebuilt in
-- bounded batches, newest first. Ineligible rows (deleted, hidden, or
-- non-user/assistant) are never queried or backfilled, so clearing
-- them is harmless and keeps the invariant that any non-NULL
-- search_tsv was produced with the current config.
UPDATE chat_messages SET search_tsv = NULL WHERE search_tsv IS NOT NULL;
