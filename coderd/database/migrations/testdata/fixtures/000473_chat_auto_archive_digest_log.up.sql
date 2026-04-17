INSERT INTO chat_auto_archive_digest_log (owner_id, last_sent_at)
SELECT
    u.id,
    '2025-01-01 00:00:00+00'
FROM users u
ORDER BY u.created_at, u.id
LIMIT 1;
