-- Estimate: what would dbpurge clean up with 30-day retention?
-- Run this read-only against dogfood. Nothing is modified.
--
-- NOTE: The file_ids column on chats doesn't exist yet (separate PR).
-- Without it, we can't know which files are referenced by which chats,
-- so we estimate file cleanup by age alone (conservative upper bound).

WITH retention AS (
    -- Change this to match your retention setting.
    SELECT 30 AS days
),
cutoff AS (
    SELECT NOW() - (days || ' days')::interval AS before_time
    FROM retention
),

-- 1. Chats eligible for deletion (archived > retention period).
deletable_chats AS (
    SELECT c.id
    FROM chats c, cutoff
    WHERE c.archived = true
      AND c.updated_at < cutoff.before_time
),

-- 2. Cascade: messages belonging to those chats.
deletable_messages AS (
    SELECT cm.id, cm.chat_id,
           pg_column_size(cm.*) AS row_bytes
    FROM chat_messages cm
    JOIN deletable_chats dc ON cm.chat_id = dc.id
),

-- 3. Cascade: diff statuses belonging to those chats.
deletable_diff_statuses AS (
    SELECT cds.chat_id,
           pg_column_size(cds.*) AS row_bytes
    FROM chat_diff_statuses cds
    JOIN deletable_chats dc ON cds.chat_id = dc.id
),

-- 4. Cascade: queued messages belonging to those chats.
deletable_queued AS (
    SELECT cqm.id,
           pg_column_size(cqm.*) AS row_bytes
    FROM chat_queued_messages cqm
    JOIN deletable_chats dc ON cqm.chat_id = dc.id
),

-- 5. Chat files older than retention period.
--    Without file_ids on chats, we can't tell which files are
--    referenced by active chats. This counts ALL old files as
--    an upper bound. The actual purge would retain files still
--    linked to active or recently-archived chats.
deletable_files AS (
    SELECT cf.id,
           pg_column_size(cf.*) AS row_bytes
    FROM chat_files cf, cutoff
    WHERE cf.created_at < cutoff.before_time
),

-- 6. Size of the chats rows themselves.
deletable_chat_sizes AS (
    SELECT c.id,
           pg_column_size(c.*) AS row_bytes
    FROM chats c
    JOIN deletable_chats dc ON c.id = dc.id
)

SELECT
    '=== PURGE ESTIMATE (retention: ' || r.days || 'd) ===' AS header,
    -- Chats (exact)
    (SELECT COUNT(*) FROM deletable_chats) AS chats_to_delete,
    pg_size_pretty((SELECT COALESCE(SUM(row_bytes), 0) FROM deletable_chat_sizes)) AS chats_size,
    -- Messages (exact — CASCADE)
    (SELECT COUNT(*) FROM deletable_messages) AS messages_to_delete,
    pg_size_pretty((SELECT COALESCE(SUM(row_bytes), 0) FROM deletable_messages)) AS messages_size,
    -- Diff statuses (exact — CASCADE)
    (SELECT COUNT(*) FROM deletable_diff_statuses) AS diff_statuses_to_delete,
    pg_size_pretty((SELECT COALESCE(SUM(row_bytes), 0) FROM deletable_diff_statuses)) AS diff_statuses_size,
    -- Queued messages (exact — CASCADE)
    (SELECT COUNT(*) FROM deletable_queued) AS queued_msgs_to_delete,
    pg_size_pretty((SELECT COALESCE(SUM(row_bytes), 0) FROM deletable_queued)) AS queued_msgs_size,
    -- Chat files (UPPER BOUND — no file_ids column yet)
    (SELECT COUNT(*) FROM deletable_files) AS files_to_delete_upper,
    pg_size_pretty((SELECT COALESCE(SUM(row_bytes), 0) FROM deletable_files)) AS files_size_upper,
    -- Totals
    pg_size_pretty(
        (SELECT COALESCE(SUM(row_bytes), 0) FROM deletable_chat_sizes) +
        (SELECT COALESCE(SUM(row_bytes), 0) FROM deletable_messages) +
        (SELECT COALESCE(SUM(row_bytes), 0) FROM deletable_diff_statuses) +
        (SELECT COALESCE(SUM(row_bytes), 0) FROM deletable_queued) +
        (SELECT COALESCE(SUM(row_bytes), 0) FROM deletable_files)
    ) AS total_size_upper,
    -- Context
    (SELECT COUNT(*) FROM chats WHERE archived = false) AS active_chats,
    (SELECT COUNT(*) FROM chats c, cutoff
     WHERE c.archived = true AND c.updated_at >= cutoff.before_time) AS recent_archived,
    (SELECT COUNT(*) FROM chat_files) AS total_files,
    (SELECT COUNT(*) FROM chat_messages) AS total_messages
FROM retention r;
