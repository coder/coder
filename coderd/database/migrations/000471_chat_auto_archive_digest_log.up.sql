CREATE TABLE chat_auto_archive_digest_log (
    owner_id     UUID        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    last_sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE chat_auto_archive_digest_log IS 'Per-owner dedupe record for the chat auto-archive digest notification. Presence of a row indicates a digest was sent to the owner; dbpurge skips re-sending until last_sent_at is older than the dedupe window (24 h).';
