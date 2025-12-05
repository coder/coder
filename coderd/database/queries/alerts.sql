-- name: FetchNewMessageMetadata :one
-- This is used to build up the alert_message's JSON payload.
SELECT at.name                                                    AS alert_name,
       at.id                                                      AS alert_template_id,
       at.actions                                                 AS actions,
       at.method                                                  AS custom_method,
       u.id                                                       AS user_id,
       u.email                                                    AS user_email,
       COALESCE(NULLIF(u.name, ''), NULLIF(u.username, ''))::text AS user_name,
       u.username                                                 AS user_username
FROM alert_templates at,
     users u
WHERE at.id = @alert_template_id
  AND u.id = @user_id;

-- name: EnqueueAlertMessage :exec
INSERT INTO alert_messages (id, alert_template_id, user_id, method, payload, targets, created_by, created_at)
VALUES (@id,
        @alert_template_id,
        @user_id,
        @method::alert_method,
        @payload::jsonb,
        @targets,
        @created_by,
        @created_at);

-- Acquires the lease for a given count of alert messages, to enable concurrent dequeuing and subsequent sending.
-- Only rows that aren't already leased (or ones which are leased but have exceeded their lease period) are returned.
--
-- A "lease" here refers to a notifier taking ownership of an alert_messages row. A lease survives for the duration
-- of CODER_ALERTS_LEASE_PERIOD. Once a message is delivered, its status is updated and the lease expires (set to NULL).
-- If a message exceeds its lease, that implies the notifier did not shutdown cleanly, or the table update failed somehow,
-- and the row will then be eligible to be dequeued by another notifier.
--
-- SKIP LOCKED is used to jump over locked rows. This prevents multiple notifiers from acquiring the same messages.
-- See: https://www.postgresql.org/docs/9.5/sql-select.html#SQL-FOR-UPDATE-SHARE
--
-- name: AcquireAlertMessages :many
WITH acquired AS (
    UPDATE
        alert_messages
            SET queued_seconds = GREATEST(0, EXTRACT(EPOCH FROM (NOW() - updated_at)))::FLOAT,
                updated_at = NOW(),
                status = 'leased'::alert_message_status,
                status_reason = 'Leased by notifier ' || sqlc.arg('notifier_id')::uuid,
                leased_until = NOW() + CONCAT(sqlc.arg('lease_seconds')::int, ' seconds')::interval
            WHERE id IN (SELECT am.id
                         FROM alert_messages AS am
                         WHERE (
                             (
                                 -- message is in acquirable states
                                 am.status IN (
                                               'pending'::alert_message_status,
                                               'temporary_failure'::alert_message_status
                                     )
                                 )
                                 -- or somehow the message was left in leased for longer than its lease period
                                 OR (
                                 am.status = 'leased'::alert_message_status
                                     AND am.leased_until < NOW()
                                 )
                             )
                           AND (
                             -- exclude all messages which have exceeded the max attempts; these will be purged later
                             am.attempt_count IS NULL OR am.attempt_count < sqlc.arg('max_attempt_count')::int
                             )
                           -- if set, do not retry until we've exceeded the wait time
                           AND (
                             CASE
                                 WHEN am.next_retry_after IS NOT NULL THEN am.next_retry_after < NOW()
                                 ELSE true
                                 END
                             )
                         ORDER BY am.created_at ASC
                                  -- Ensure that multiple concurrent readers cannot retrieve the same rows
                             FOR UPDATE OF am
                                 SKIP LOCKED
                         LIMIT sqlc.arg('count'))
            RETURNING *)
SELECT
    -- message
    am.id,
    am.payload,
    am.method,
    am.attempt_count::int                                                 AS attempt_count,
    am.queued_seconds::float                                              AS queued_seconds,
    -- template
    at.id                                                                 AS template_id,
    at.title_template,
    at.body_template,
    -- preferences
    (CASE WHEN ap.disabled IS NULL THEN false ELSE ap.disabled END)::bool AS disabled
FROM acquired am
         JOIN alert_templates at ON am.alert_template_id = at.id
         LEFT JOIN alert_preferences AS ap
                   ON (ap.user_id = am.user_id AND ap.alert_template_id = am.alert_template_id);

-- name: BulkMarkAlertMessagesFailed :execrows
UPDATE alert_messages
SET queued_seconds   = 0,
    updated_at       = subquery.failed_at,
    attempt_count    = attempt_count + 1,
    status           = CASE
                           WHEN attempt_count + 1 < @max_attempts::int THEN subquery.status
                           ELSE 'permanent_failure'::alert_message_status END,
    status_reason    = subquery.status_reason,
    leased_until     = NULL,
    next_retry_after = CASE
                           WHEN (attempt_count + 1 < @max_attempts::int)
                               THEN NOW() + CONCAT(@retry_interval::int, ' seconds')::interval END
FROM (SELECT UNNEST(@ids::uuid[])                      AS id,
             UNNEST(@failed_ats::timestamptz[])        AS failed_at,
             UNNEST(@statuses::alert_message_status[]) AS status,
             UNNEST(@status_reasons::text[])           AS status_reason) AS subquery
WHERE alert_messages.id = subquery.id;

-- name: BulkMarkAlertMessagesSent :execrows
UPDATE alert_messages
SET queued_seconds   = 0,
    updated_at       = new_values.sent_at,
    attempt_count    = attempt_count + 1,
    status           = 'sent'::alert_message_status,
    status_reason    = NULL,
    leased_until     = NULL,
    next_retry_after = NULL
FROM (SELECT UNNEST(@ids::uuid[])             AS id,
             UNNEST(@sent_ats::timestamptz[]) AS sent_at)
         AS new_values
WHERE alert_messages.id = new_values.id;

-- Delete all alert messages which have not been updated for over a week.
-- name: DeleteOldAlertMessages :exec
DELETE
FROM alert_messages
WHERE id IN
      (SELECT id
       FROM alert_messages AS nested
       WHERE nested.updated_at < NOW() - INTERVAL '7 days');

-- name: GetAlertMessagesByStatus :many
SELECT *
FROM alert_messages
WHERE status = @status
LIMIT sqlc.arg('limit')::int;

-- name: GetUserAlertPreferences :many
SELECT *
FROM alert_preferences
WHERE user_id = @user_id::uuid;

-- name: UpdateUserAlertPreferences :execrows
INSERT
INTO alert_preferences (user_id, alert_template_id, disabled)
SELECT @user_id::uuid, new_values.alert_template_id, new_values.disabled
FROM (SELECT UNNEST(@alert_template_ids::uuid[]) AS alert_template_id,
             UNNEST(@disableds::bool[])          AS disabled) AS new_values
ON CONFLICT (user_id, alert_template_id) DO UPDATE
    SET disabled   = EXCLUDED.disabled,
        updated_at = CURRENT_TIMESTAMP;

-- name: UpdateAlertTemplateMethodByID :one
UPDATE alert_templates
SET method = sqlc.narg('method')::alert_method
WHERE id = @id::uuid
RETURNING *;

-- name: GetAlertTemplateByID :one
SELECT *
FROM alert_templates
WHERE id = @id::uuid;

-- name: GetAlertTemplatesByKind :many
SELECT *
FROM alert_templates
WHERE kind = @kind::alert_template_kind
ORDER BY name ASC;

-- name: GetAlertReportGeneratorLogByTemplate :one
-- Fetch the alert report generator log indicating recent activity.
SELECT
	*
FROM
	alert_report_generator_logs
WHERE
	alert_template_id = @template_id::uuid;

-- name: UpsertAlertReportGeneratorLog :exec
-- Insert or update alert report generator logs with recent activity.
INSERT INTO alert_report_generator_logs (alert_template_id, last_generated_at) VALUES (@alert_template_id, @last_generated_at)
ON CONFLICT (alert_template_id) DO UPDATE set last_generated_at = EXCLUDED.last_generated_at
WHERE alert_report_generator_logs.alert_template_id = EXCLUDED.alert_template_id;

-- name: GetWebpushSubscriptionsByUserID :many
SELECT *
FROM webpush_subscriptions
WHERE user_id = @user_id::uuid;

-- name: InsertWebpushSubscription :one
INSERT INTO webpush_subscriptions (user_id, created_at, endpoint, endpoint_p256dh_key, endpoint_auth_key)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: DeleteWebpushSubscriptions :exec
DELETE FROM webpush_subscriptions
WHERE id = ANY(@ids::uuid[]);

-- name: DeleteWebpushSubscriptionByUserIDAndEndpoint :exec
DELETE FROM webpush_subscriptions
WHERE user_id = @user_id AND endpoint = @endpoint;

-- name: DeleteAllWebpushSubscriptions :exec
-- Deletes all existing webpush subscriptions.
-- This should be called when the VAPID keypair is regenerated, as the old
-- keypair will no longer be valid and all existing subscriptions will need to
-- be recreated.
TRUNCATE TABLE webpush_subscriptions;
