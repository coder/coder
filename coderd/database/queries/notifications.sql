-- name: FetchNewMessageMetadata :one
-- This is used to build up the notification_message's JSON payload.
SELECT nt.name                                                    AS notification_name,
       nt.id                                                      AS notification_template_id,
       nt.actions                                                 AS actions,
       nt.method                                                  AS custom_method,
       u.id                                                       AS user_id,
       u.email                                                    AS user_email,
       COALESCE(NULLIF(u.name, ''), NULLIF(u.username, ''))::text AS user_name,
       u.username                                                 AS user_username
FROM notification_templates nt,
     users u
WHERE nt.id = @notification_template_id
  AND u.id = @user_id;

-- name: EnqueueNotificationMessage :exec
INSERT INTO notification_messages (id, notification_template_id, user_id, method, payload, targets, created_by, created_at)
VALUES (@id,
        @notification_template_id,
        @user_id,
        @method::notification_method,
        @payload::jsonb,
        @targets,
        @created_by,
        @created_at);

-- Acquires the lease for a given count of notification messages, to enable concurrent dequeuing and subsequent sending.
-- Only rows that aren't already leased (or ones which are leased but have exceeded their lease period) are returned.
--
-- A "lease" here refers to a notifier taking ownership of a notification_messages row. A lease survives for the duration
-- of CODER_NOTIFICATIONS_LEASE_PERIOD. Once a message is delivered, its status is updated and the lease expires (set to NULL).
-- If a message exceeds its lease, that implies the notifier did not shutdown cleanly, or the table update failed somehow,
-- and the row will then be eligible to be dequeued by another notifier.
--
-- SKIP LOCKED is used to jump over locked rows. This prevents multiple notifiers from acquiring the same messages.
-- See: https://www.postgresql.org/docs/9.5/sql-select.html#SQL-FOR-UPDATE-SHARE
--
-- name: AcquireNotificationMessages :many
WITH acquired AS (
    UPDATE
        notification_messages
            SET queued_seconds = GREATEST(0, EXTRACT(EPOCH FROM (NOW() - updated_at)))::FLOAT,
                updated_at = NOW(),
                status = 'leased'::notification_message_status,
                status_reason = 'Leased by notifier ' || sqlc.arg('notifier_id')::uuid,
                leased_until = NOW() + CONCAT(sqlc.arg('lease_seconds')::int, ' seconds')::interval
            WHERE id IN (SELECT nm.id
                         FROM notification_messages AS nm
                         WHERE (
                             (
                                 -- message is in acquirable states
                                 nm.status IN (
                                               'pending'::notification_message_status,
                                               'temporary_failure'::notification_message_status
                                     )
                                 )
                                 -- or somehow the message was left in leased for longer than its lease period
                                 OR (
                                 nm.status = 'leased'::notification_message_status
                                     AND nm.leased_until < NOW()
                                 )
                             )
                           AND (
                             -- exclude all messages which have exceeded the max attempts; these will be purged later
                             nm.attempt_count IS NULL OR nm.attempt_count < sqlc.arg('max_attempt_count')::int
                             )
                           -- if set, do not retry until we've exceeded the wait time
                           AND (
                             CASE
                                 WHEN nm.next_retry_after IS NOT NULL THEN nm.next_retry_after < NOW()
                                 ELSE true
                                 END
                             )
                         ORDER BY nm.created_at ASC
                                  -- Ensure that multiple concurrent readers cannot retrieve the same rows
                             FOR UPDATE OF nm
                                 SKIP LOCKED
                         LIMIT sqlc.arg('count'))
            RETURNING *)
SELECT
    -- message
    nm.id,
    nm.payload,
    nm.method,
    nm.attempt_count::int                                                 AS attempt_count,
    nm.queued_seconds::float                                              AS queued_seconds,
	nm.targets,
    -- template
    nt.id                                                                 AS template_id,
    nt.title_template,
    nt.body_template,
    -- preferences
    (CASE WHEN np.disabled IS NULL THEN false ELSE np.disabled END)::bool AS disabled
FROM acquired nm
         JOIN notification_templates nt ON nm.notification_template_id = nt.id
         LEFT JOIN notification_preferences AS np
                   ON (np.user_id = nm.user_id AND np.notification_template_id = nm.notification_template_id);

-- name: BulkMarkNotificationMessagesFailed :execrows
UPDATE notification_messages
SET queued_seconds   = 0,
    updated_at       = subquery.failed_at,
    attempt_count    = attempt_count + 1,
    status           = CASE
                           WHEN attempt_count + 1 < @max_attempts::int THEN subquery.status
                           ELSE 'permanent_failure'::notification_message_status END,
    status_reason    = subquery.status_reason,
    leased_until     = NULL,
    next_retry_after = CASE
                           WHEN (attempt_count + 1 < @max_attempts::int)
                               THEN NOW() + CONCAT(@retry_interval::int, ' seconds')::interval END
FROM (SELECT UNNEST(@ids::uuid[])                             AS id,
             UNNEST(@failed_ats::timestamptz[])               AS failed_at,
             UNNEST(@statuses::notification_message_status[]) AS status,
             UNNEST(@status_reasons::text[])                  AS status_reason) AS subquery
WHERE notification_messages.id = subquery.id;

-- name: BulkMarkNotificationMessagesSent :execrows
UPDATE notification_messages
SET queued_seconds   = 0,
    updated_at       = new_values.sent_at,
    attempt_count    = attempt_count + 1,
    status           = 'sent'::notification_message_status,
    status_reason    = NULL,
    leased_until     = NULL,
    next_retry_after = NULL
FROM (SELECT UNNEST(@ids::uuid[])             AS id,
             UNNEST(@sent_ats::timestamptz[]) AS sent_at)
         AS new_values
WHERE notification_messages.id = new_values.id;

-- Delete all notification messages which have not been updated for over a week.
-- name: DeleteOldNotificationMessages :exec
DELETE
FROM notification_messages
WHERE id IN
      (SELECT id
       FROM notification_messages AS nested
       WHERE nested.updated_at < NOW() - INTERVAL '7 days');

-- name: GetNotificationMessagesByStatus :many
SELECT *
FROM notification_messages
WHERE status = @status
LIMIT sqlc.arg('limit')::int;

-- name: GetUserNotificationPreferences :many
SELECT *
FROM notification_preferences
WHERE user_id = @user_id::uuid;

-- name: UpdateUserNotificationPreferences :execrows
INSERT
INTO notification_preferences (user_id, notification_template_id, disabled)
SELECT @user_id::uuid, new_values.notification_template_id, new_values.disabled
FROM (SELECT UNNEST(@notification_template_ids::uuid[]) AS notification_template_id,
             UNNEST(@disableds::bool[])                 AS disabled) AS new_values
ON CONFLICT (user_id, notification_template_id) DO UPDATE
    SET disabled   = EXCLUDED.disabled,
        updated_at = CURRENT_TIMESTAMP;

-- name: UpdateNotificationTemplateMethodByID :one
UPDATE notification_templates
SET method = sqlc.narg('method')::notification_method
WHERE id = @id::uuid
RETURNING *;

-- name: GetNotificationTemplateByID :one
SELECT *
FROM notification_templates
WHERE id = @id::uuid;

-- name: GetNotificationTemplatesByKind :many
SELECT *
FROM notification_templates
WHERE kind = @kind::notification_template_kind
ORDER BY name ASC;

-- name: GetNotificationReportGeneratorLogByTemplate :one
-- Fetch the notification report generator log indicating recent activity.
SELECT
	*
FROM
	notification_report_generator_logs
WHERE
	notification_template_id = @template_id::uuid;

-- name: UpsertNotificationReportGeneratorLog :exec
-- Insert or update notification report generator logs with recent activity.
INSERT INTO notification_report_generator_logs (notification_template_id, last_generated_at) VALUES (@notification_template_id, @last_generated_at)
ON CONFLICT (notification_template_id) DO UPDATE set last_generated_at = EXCLUDED.last_generated_at
WHERE notification_report_generator_logs.notification_template_id = EXCLUDED.notification_template_id;
