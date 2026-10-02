-- name: InsertChatAutomation :one
INSERT INTO chat_automations (
    id,
    organization_id,
    owner_id,
    name,
    created_by_chat_id,
    kind,
    enabled,
    target_mode,
    target_chat_id,
    new_chat_model_config_id,
    reasoning_effort,
    when_busy,
    webhook_use,
    webhook_secret_hash,
    webhook_secret_version,
    prompt,
    schedule_cron,
    schedule_time_zone,
    schedule_next_run_at,
    created_at,
    updated_at
) VALUES (
    @id,
    @organization_id,
    @owner_id,
    @name,
    @created_by_chat_id,
    @kind,
    @enabled,
    @target_mode,
    @target_chat_id,
    @new_chat_model_config_id,
    @reasoning_effort,
    @when_busy,
    @webhook_use,
    -- A nil []byte is sent as an empty bytea, not NULL. Store NULL so
    -- automations without a secret satisfy chat_automations_kind_shape.
    NULLIF(@webhook_secret_hash::bytea, ''::bytea),
    @webhook_secret_version,
    @prompt,
    @schedule_cron,
    @schedule_time_zone,
    @schedule_next_run_at,
    @created_at,
    @updated_at
)
RETURNING
    *;

-- name: GetChatAutomationByID :one
SELECT
    *
FROM
    chat_automations
WHERE
    id = @id::uuid;

-- name: DeleteChatAutomationByID :exec
DELETE FROM
    chat_automations
WHERE
    id = @id::uuid;

-- name: GetChatAutomationsByIDsForUpdate :many
-- Locks the given automations in ascending id order so concurrent
-- lockers always acquire automation row locks in the same order.
-- Missing ids are not returned.
SELECT
    *
FROM
    chat_automations
WHERE
    id = ANY(@ids::uuid[])
ORDER BY
    id
FOR UPDATE;

-- name: GetChatAutomationsByOrganizationID :many
SELECT
    *
FROM
    chat_automations
WHERE
    organization_id = @organization_id::uuid
ORDER BY
    created_at DESC,
    id DESC;

-- name: GetChatAutomationsByOrganizationIDAndOwnerID :many
SELECT
    *
FROM
    chat_automations
WHERE
    organization_id = @organization_id::uuid
    AND owner_id = @owner_id::uuid
ORDER BY
    created_at DESC,
    id DESC;

-- name: CountChatAutomationsByOwnerID :one
-- Counts the automations owner_id owns across all organizations.
SELECT
    COUNT(*)
FROM
    chat_automations
WHERE
    owner_id = @owner_id::uuid;

-- name: UpdateChatAutomationByID :one
UPDATE
    chat_automations
SET
    name = @name,
    prompt = @prompt,
    target_chat_id = @target_chat_id,
    new_chat_model_config_id = @new_chat_model_config_id,
    reasoning_effort = @reasoning_effort,
    when_busy = @when_busy,
    schedule_cron = @schedule_cron,
    schedule_time_zone = @schedule_time_zone,
    schedule_revision = @schedule_revision,
    schedule_next_run_at = @schedule_next_run_at,
    enabled = @enabled,
    queue_generation = @queue_generation,
    updated_at = @updated_at
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: UpdateChatAutomationWebhookSecretByID :one
-- Replaces the webhook secret hash and increments the secret version. The
-- single-use marker webhook_consumed_at is intentionally kept.
UPDATE
    chat_automations
SET
    webhook_secret_hash = @webhook_secret_hash,
    webhook_secret_version = webhook_secret_version + 1,
    updated_at = @updated_at
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: ConsumeChatAutomationWebhookByID :execrows
-- Marks an unconsumed single-use webhook as consumed. It affects no row
-- when the automation is not a single-use webhook or was already
-- consumed, so callers can refuse the delivery.
UPDATE
    chat_automations
SET
    webhook_consumed_at = @now::timestamptz,
    updated_at = @now::timestamptz
WHERE
    id = @id::uuid
    AND kind = 'webhook'
    AND webhook_use = 'single'
    AND webhook_consumed_at IS NULL;

-- name: GetDueChatAutomationSchedules :many
-- Returns enabled schedule automations whose cursor is at or before now,
-- oldest cursor first, starting after the (after_next_run_at, after_id)
-- keyset so callers can page through every due row. Automations of
-- inactive owners and existing_chat automations whose target chat is gone
-- or archived are left out. It takes no locks: publishing rechecks each
-- row under the chat and automation locks.
SELECT
    chat_automations.*
FROM
    chat_automations
    JOIN users ON users.id = chat_automations.owner_id
    LEFT JOIN chats ON chats.id = chat_automations.target_chat_id
WHERE
    chat_automations.kind = 'schedule'
    AND chat_automations.enabled
    AND chat_automations.schedule_next_run_at <= @now::timestamptz
    AND (chat_automations.schedule_next_run_at, chat_automations.id) > (@after_next_run_at::timestamptz, @after_id::uuid)
    AND users.status = 'active'
    AND NOT users.deleted
    AND (
        chat_automations.target_mode = 'new_chat'
        OR (chats.id IS NOT NULL AND NOT chats.archived)
    )
ORDER BY
    chat_automations.schedule_next_run_at,
    chat_automations.id
LIMIT
    @limit_count::int;

-- name: AdvanceChatAutomationScheduleCursor :execrows
-- Moves the schedule cursor of an enabled schedule automation from the
-- observed occurrence to next_run_at. It affects no row when the schedule
-- revision or the cursor changed since they were observed, so exactly one
-- caller moves the cursor past each occurrence. A NULL next_run_at means
-- no occurrence is pending.
UPDATE
    chat_automations
SET
    schedule_next_run_at = sqlc.narg('next_run_at')::timestamptz,
    updated_at = @updated_at::timestamptz
WHERE
    id = @id::uuid
    AND kind = 'schedule'
    AND enabled
    AND schedule_revision = @schedule_revision::bigint
    AND schedule_next_run_at = @observed_next_run_at::timestamptz;

-- name: GetChatAutomationReferencesByChatID :many
-- Returns the id, name and kind of each automation that delivered into
-- the chat: the automation that created the chat, the automations of its
-- visible non-deleted user messages and the automations of its queued
-- messages. Only automations in the chat's organization are returned, and
-- deleted automations are absent.
WITH referenced AS (
    SELECT
        chats.automation_id
    FROM
        chats
    WHERE
        chats.id = @chat_id::uuid
        AND chats.automation_id IS NOT NULL
    UNION
    -- The role, deleted and visibility predicates match
    -- idx_chat_messages_user_prompts so the index applies.
    SELECT
        chat_messages.automation_id
    FROM
        chat_messages
    WHERE
        chat_messages.chat_id = @chat_id::uuid
        AND chat_messages.deleted = false
        AND chat_messages.role = 'user'
        AND chat_messages.visibility IN ('user', 'both')
        AND chat_messages.automation_id IS NOT NULL
    UNION
    SELECT
        chat_queued_messages.automation_id
    FROM
        chat_queued_messages
    WHERE
        chat_queued_messages.chat_id = @chat_id::uuid
        AND chat_queued_messages.automation_id IS NOT NULL
)
SELECT
    chat_automations.id,
    chat_automations.name,
    chat_automations.kind
FROM
    referenced
    JOIN chat_automations ON chat_automations.id = referenced.automation_id
    JOIN chats ON chats.id = @chat_id::uuid
WHERE
    chat_automations.organization_id = chats.organization_id
ORDER BY
    chat_automations.id;
