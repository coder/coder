-- name: GetChatAutoArchiveDigestLogsForOwners :many
-- Returns the last-sent timestamp for each requested owner. Owners
-- without a row are simply absent from the result; callers treat
-- them as "never sent". Used by dbpurge to decide whether to skip
-- a digest enqueue during the 24 h dedupe window.
SELECT owner_id, last_sent_at
FROM chat_auto_archive_digest_log
WHERE owner_id = ANY(@owner_ids::uuid[]);

-- name: UpsertChatAutoArchiveDigestLog :exec
-- Records that we sent (or attempted to send) a chat auto-archive
-- digest to the given owner at the given timestamp. Written AFTER
-- the notification enqueue succeeds so an enqueue failure allows a
-- retry on the next tick.
INSERT INTO chat_auto_archive_digest_log (owner_id, last_sent_at)
VALUES (@owner_id::uuid, @last_sent_at::timestamptz)
ON CONFLICT (owner_id) DO UPDATE
    SET last_sent_at = EXCLUDED.last_sent_at;
