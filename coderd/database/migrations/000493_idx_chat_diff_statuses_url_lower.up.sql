-- Index on LOWER(url) supports case-insensitive lookups when filtering
-- chats by their associated diff URL (e.g. a pull request URL).
CREATE INDEX idx_chat_diff_statuses_url_lower
    ON chat_diff_statuses (LOWER(url))
    WHERE url IS NOT NULL AND url <> '';
