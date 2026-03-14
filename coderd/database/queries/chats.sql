-- name: ArchiveChatByID :exec
UPDATE chats SET archived = true, updated_at = NOW()
WHERE id = @id OR root_chat_id = @id;

-- name: UnarchiveChatByID :exec
UPDATE chats SET archived = false, updated_at = NOW() WHERE id = @id::uuid;

-- name: DeleteChatMessagesByChatID :exec
DELETE FROM
    chat_messages
WHERE
    chat_id = @chat_id::uuid;

-- name: DeleteChatMessagesAfterID :exec
DELETE FROM
    chat_messages
WHERE
    chat_id = @chat_id::uuid
    AND id > @after_id::bigint;

-- name: GetChatByID :one
SELECT
    *
FROM
    chats
WHERE
    id = @id::uuid;

-- name: GetChatWithStatusByID :one
SELECT
    *
FROM
    chats_with_status
WHERE
    id = @id::uuid;

-- name: GetChatMessageByID :one
SELECT
    *
FROM
    chat_messages
WHERE
    id = @id::bigint;

-- name: GetChatMessagesByChatID :many
SELECT
    *
FROM
    chat_messages
WHERE
    chat_id = @chat_id::uuid
    AND id > @after_id::bigint
    AND visibility IN ('user', 'both')
ORDER BY
    created_at ASC;

-- name: GetChatMessagesForPromptByChatID :many
WITH latest_compressed_summary AS (
    SELECT
        id
    FROM
        chat_messages
    WHERE
        chat_id = @chat_id::uuid
        AND compressed = TRUE
        AND visibility = 'model'
    ORDER BY
        created_at DESC,
        id DESC
    LIMIT
        1
)
SELECT
    *
FROM
    chat_messages
WHERE
    chat_id = @chat_id::uuid
    AND visibility IN ('model', 'both')
    AND (
        (
            role = 'system'
            AND compressed = FALSE
        )
        OR (
            compressed = FALSE
            AND (
                NOT EXISTS (
                    SELECT
                        1
                    FROM
                        latest_compressed_summary
                )
                OR id > (
                    SELECT
                        id
                    FROM
                        latest_compressed_summary
                )
            )
        )
        OR id = (
            SELECT
                id
            FROM
                latest_compressed_summary
        )
    )
ORDER BY
    created_at ASC,
    id ASC;

-- name: GetChatsByOwnerID :many
SELECT
    *
FROM
    chats
WHERE
    owner_id = @owner_id::uuid
    AND CASE
        WHEN sqlc.narg('archived') :: boolean IS NULL THEN true
        ELSE chats.archived = sqlc.narg('archived') :: boolean
    END
    AND CASE
        -- This allows using the last element on a page as effectively a cursor.
        -- This is an important option for scripts that need to paginate without
        -- duplicating or missing data.
        WHEN @after_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN (
            -- The pagination cursor is the last ID of the previous page.
            -- The query is ordered by the updated_at field, so select all
            -- rows before the cursor.
            (updated_at, id) < (
                SELECT
                    updated_at, id
                FROM
                    chats
                WHERE
                    id = @after_id
            )
        )
        ELSE true
    END
ORDER BY
    -- Deterministic and consistent ordering of all rows, even if they share
    -- a timestamp. This is to ensure consistent pagination.
    (updated_at, id) DESC OFFSET @offset_opt
LIMIT
    -- The chat list is unbounded and expected to grow large.
    -- Default to 50 to prevent accidental excessively large queries.
    COALESCE(NULLIF(@limit_opt :: int, 0), 50);

-- name: GetChatsWithStatusByOwnerID :many
SELECT
    *
FROM
    chats_with_status
WHERE
    owner_id = @owner_id::uuid
    AND CASE
        WHEN sqlc.narg('archived') :: boolean IS NULL THEN true
        ELSE chats_with_status.archived = sqlc.narg('archived') :: boolean
    END
    AND CASE
        WHEN @after_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN (
            (updated_at, id) < (
                SELECT
                    updated_at, id
                FROM
                    chats
                WHERE
                    id = @after_id
            )
        )
        ELSE true
    END
ORDER BY
    (updated_at, id) DESC OFFSET @offset_opt
LIMIT
    COALESCE(NULLIF(@limit_opt :: int, 0), 50);

-- name: ListChildChatsByParentID :many
SELECT
    *
FROM
    chats
WHERE
    parent_chat_id = @parent_chat_id::uuid
ORDER BY
    created_at ASC;

-- name: ListChatsByRootID :many
SELECT
    *
FROM
    chats
WHERE
    root_chat_id = @root_chat_id::uuid
ORDER BY
    created_at ASC;

-- name: InsertChat :one
INSERT INTO chats (
    owner_id,
    workspace_id,
    parent_chat_id,
    root_chat_id,
    last_model_config_id,
    title,
    mode
) VALUES (
    @owner_id::uuid,
    sqlc.narg('workspace_id')::uuid,
    sqlc.narg('parent_chat_id')::uuid,
    sqlc.narg('root_chat_id')::uuid,
    @last_model_config_id::uuid,
    @title::text,
    sqlc.narg('mode')::chat_mode
)
RETURNING
    *;

-- name: InsertChatMessage :one
WITH updated_chat AS (
    UPDATE
        chats
    SET
        last_model_config_id = sqlc.narg('model_config_id')::uuid
    WHERE
        id = @chat_id::uuid
        AND sqlc.narg('model_config_id')::uuid IS NOT NULL
)
INSERT INTO chat_messages (
    chat_id,
    created_by,
    model_config_id,
    chat_run_id,
    chat_run_step_id,
    role,
    content,
    content_version,
    visibility,
    compressed
) VALUES (
    @chat_id::uuid,
    sqlc.narg('created_by')::uuid,
    sqlc.narg('model_config_id')::uuid,
    sqlc.narg('chat_run_id')::uuid,
    sqlc.narg('chat_run_step_id')::uuid,
    @role::chat_message_role,
    sqlc.narg('content')::jsonb,
    @content_version::smallint,
    @visibility::chat_message_visibility,
    COALESCE(sqlc.narg('compressed')::boolean, FALSE)
)
RETURNING
    *;

-- name: UpdateChatMessageByID :one
UPDATE
    chat_messages
SET
    model_config_id = COALESCE(sqlc.narg('model_config_id')::uuid, model_config_id),
    content = sqlc.narg('content')::jsonb
WHERE
    id = @id::bigint
RETURNING
    *;

-- name: UpdateChatByID :one
UPDATE
    chats
SET
    title = @title::text,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: UpdateChatWorkspace :one
UPDATE
    chats
SET
    workspace_id = sqlc.narg('workspace_id')::uuid,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: GetChatDiffStatusByChatID :one
SELECT
    *
FROM
    chat_diff_statuses
WHERE
    chat_id = @chat_id::uuid;

-- name: GetChatDiffStatusesByChatIDs :many
SELECT
    *
FROM
    chat_diff_statuses
WHERE
    chat_id = ANY(@chat_ids::uuid[]);

-- name: UpsertChatDiffStatusReference :one
INSERT INTO chat_diff_statuses (
    chat_id,
    url,
    git_branch,
    git_remote_origin,
    stale_at
) VALUES (
    @chat_id::uuid,
    sqlc.narg('url')::text,
    @git_branch::text,
    @git_remote_origin::text,
    @stale_at::timestamptz
)
ON CONFLICT (chat_id) DO UPDATE
SET
    url = CASE
        WHEN EXCLUDED.url IS NOT NULL THEN EXCLUDED.url
        ELSE chat_diff_statuses.url
    END,
    git_branch = CASE
        WHEN EXCLUDED.git_branch != '' THEN EXCLUDED.git_branch
        ELSE chat_diff_statuses.git_branch
    END,
    git_remote_origin = CASE
        WHEN EXCLUDED.git_remote_origin != '' THEN EXCLUDED.git_remote_origin
        ELSE chat_diff_statuses.git_remote_origin
    END,
    stale_at = EXCLUDED.stale_at,
    updated_at = NOW()
RETURNING
    *;

-- name: UpsertChatDiffStatus :one
INSERT INTO chat_diff_statuses (
    chat_id,
    url,
    pull_request_state,
    pull_request_title,
    pull_request_draft,
    changes_requested,
    additions,
    deletions,
    changed_files,
    author_login,
    author_avatar_url,
    base_branch,
    pr_number,
    commits,
    approved,
    reviewer_count,
    refreshed_at,
    stale_at
) VALUES (
    @chat_id::uuid,
    sqlc.narg('url')::text,
    sqlc.narg('pull_request_state')::text,
    @pull_request_title::text,
    @pull_request_draft::boolean,
    @changes_requested::boolean,
    @additions::integer,
    @deletions::integer,
    @changed_files::integer,
    sqlc.narg('author_login')::text,
    sqlc.narg('author_avatar_url')::text,
    sqlc.narg('base_branch')::text,
    sqlc.narg('pr_number')::integer,
    sqlc.narg('commits')::integer,
    sqlc.narg('approved')::boolean,
    sqlc.narg('reviewer_count')::integer,
    @refreshed_at::timestamptz,
    @stale_at::timestamptz
)
ON CONFLICT (chat_id) DO UPDATE
SET
    url = EXCLUDED.url,
    pull_request_state = EXCLUDED.pull_request_state,
    pull_request_title = EXCLUDED.pull_request_title,
    pull_request_draft = EXCLUDED.pull_request_draft,
    changes_requested = EXCLUDED.changes_requested,
    additions = EXCLUDED.additions,
    deletions = EXCLUDED.deletions,
    changed_files = EXCLUDED.changed_files,
    author_login = EXCLUDED.author_login,
    author_avatar_url = EXCLUDED.author_avatar_url,
    base_branch = EXCLUDED.base_branch,
    pr_number = EXCLUDED.pr_number,
    commits = EXCLUDED.commits,
    approved = EXCLUDED.approved,
    reviewer_count = EXCLUDED.reviewer_count,
    refreshed_at = EXCLUDED.refreshed_at,
    stale_at = EXCLUDED.stale_at,
    updated_at = NOW()
RETURNING
    *;

-- name: InsertChatQueuedMessage :one
INSERT INTO chat_queued_messages (chat_id, content)
VALUES (@chat_id, @content)
RETURNING *;

-- name: GetChatQueuedMessages :many
SELECT * FROM chat_queued_messages
WHERE chat_id = @chat_id
ORDER BY id ASC;

-- name: DeleteChatQueuedMessage :exec
DELETE FROM chat_queued_messages WHERE id = @id AND chat_id = @chat_id;

-- name: DeleteAllChatQueuedMessages :exec
DELETE FROM chat_queued_messages WHERE chat_id = @chat_id;

-- name: PopNextQueuedMessage :one
DELETE FROM chat_queued_messages
WHERE id = (
    SELECT cqm.id FROM chat_queued_messages cqm
    WHERE cqm.chat_id = @chat_id
    ORDER BY cqm.id ASC
    LIMIT 1
)
RETURNING *;

-- name: GetLastChatMessageByRole :one
SELECT
    *
FROM
    chat_messages
WHERE
    chat_id = @chat_id::uuid
    AND role = @role::chat_message_role
ORDER BY
    created_at DESC, id DESC
LIMIT
    1;

-- name: GetChatByIDForUpdate :one
SELECT * FROM chats WHERE id = @id::uuid FOR UPDATE;

-- name: AcquireStaleChatDiffStatuses :many
WITH acquired AS (
    UPDATE
        chat_diff_statuses
    SET
        -- Claim for 5 minutes. The worker sets the real stale_at
        -- after refresh. If the worker crashes, rows become eligible
        -- again after this interval.
        stale_at = NOW() + INTERVAL '5 minutes',
        updated_at = NOW()
    WHERE
        chat_id IN (
            SELECT
                cds.chat_id
            FROM
                chat_diff_statuses cds
            INNER JOIN
                chats c ON c.id = cds.chat_id
            WHERE
                cds.stale_at <= NOW()
                AND cds.git_remote_origin != ''
                AND cds.git_branch != ''
                AND c.archived = FALSE
            ORDER BY
                cds.stale_at ASC
            FOR UPDATE OF cds
                SKIP LOCKED
            LIMIT
                @limit_val::int
        )
    RETURNING *
)
SELECT
    acquired.*,
    c.owner_id
FROM
    acquired
INNER JOIN
    chats c ON c.id = acquired.chat_id;

-- name: BackoffChatDiffStatus :exec
UPDATE
    chat_diff_statuses
SET
    stale_at = @stale_at::timestamptz,
    updated_at = NOW()
WHERE
    chat_id = @chat_id::uuid;

-- name: GetChatCostSummary :one
-- Aggregate cost summary for a single user within a date range.
-- Reads from chat_run_steps (the immutable token/cost audit trail).
SELECT
    COALESCE(SUM(s.total_cost_micros), 0)::bigint AS total_cost_micros,
    COUNT(*) FILTER (
        WHERE s.total_cost_micros IS NOT NULL
    )::bigint AS priced_step_count,
    COUNT(*) FILTER (
        WHERE s.total_cost_micros IS NULL
            AND (
                s.input_tokens IS NOT NULL
                OR s.output_tokens IS NOT NULL
                OR s.reasoning_tokens IS NOT NULL
                OR s.cache_creation_tokens IS NOT NULL
                OR s.cache_read_tokens IS NOT NULL
            )
    )::bigint AS unpriced_step_count,
    COALESCE(SUM(s.input_tokens), 0)::bigint AS total_input_tokens,
    COALESCE(SUM(s.output_tokens), 0)::bigint AS total_output_tokens,
    COALESCE(SUM(s.cache_read_tokens), 0)::bigint AS total_cache_read_tokens,
    COALESCE(SUM(s.cache_creation_tokens), 0)::bigint AS total_cache_creation_tokens
FROM
    chat_run_steps s
JOIN
    chat_runs r ON r.id = s.chat_run_id
JOIN
    chats c ON c.id = r.chat_id
WHERE
    c.owner_id = @owner_id::uuid
    AND s.completed_at IS NOT NULL
    AND s.started_at >= @start_date::timestamptz
    AND s.started_at < @end_date::timestamptz;

-- name: GetChatCostPerModel :many
-- Per-model cost breakdown for a single user within a date range.
-- Reads from chat_run_steps grouped by model config.
SELECT
    cmc.id AS model_config_id,
    cmc.display_name,
    cmc.provider,
    cmc.model,
    COALESCE(SUM(s.total_cost_micros), 0)::bigint AS total_cost_micros,
    COUNT(*)::bigint AS step_count,
    COALESCE(SUM(s.input_tokens), 0)::bigint AS total_input_tokens,
    COALESCE(SUM(s.output_tokens), 0)::bigint AS total_output_tokens,
    COALESCE(SUM(s.cache_read_tokens), 0)::bigint AS total_cache_read_tokens,
    COALESCE(SUM(s.cache_creation_tokens), 0)::bigint AS total_cache_creation_tokens
FROM
    chat_run_steps s
JOIN
    chat_runs r ON r.id = s.chat_run_id
JOIN
    chats c ON c.id = r.chat_id
JOIN
    chat_model_configs cmc ON cmc.id = s.model_config_id
WHERE
    c.owner_id = @owner_id::uuid
    AND s.completed_at IS NOT NULL
    AND s.started_at >= @start_date::timestamptz
    AND s.started_at < @end_date::timestamptz
GROUP BY
    cmc.id, cmc.display_name, cmc.provider, cmc.model
ORDER BY
    total_cost_micros DESC;

-- name: GetChatCostPerChat :many
-- Per-root-chat cost breakdown for a single user within a date range.
-- Groups by root_chat_id so forked chats roll up under their root.
WITH chat_costs AS (
    SELECT
        COALESCE(c.root_chat_id, c.id) AS root_chat_id,
        COALESCE(SUM(s.total_cost_micros), 0)::bigint AS total_cost_micros,
        COUNT(*)::bigint AS step_count,
        COALESCE(SUM(s.input_tokens), 0)::bigint AS total_input_tokens,
        COALESCE(SUM(s.output_tokens), 0)::bigint AS total_output_tokens,
        COALESCE(SUM(s.cache_read_tokens), 0)::bigint AS total_cache_read_tokens,
        COALESCE(SUM(s.cache_creation_tokens), 0)::bigint AS total_cache_creation_tokens
    FROM chat_run_steps s
    JOIN chat_runs r ON r.id = s.chat_run_id
    JOIN chats c ON c.id = r.chat_id
    WHERE c.owner_id = @owner_id::uuid
      AND s.completed_at IS NOT NULL
      AND s.started_at >= @start_date::timestamptz
      AND s.started_at < @end_date::timestamptz
    GROUP BY COALESCE(c.root_chat_id, c.id)
)
SELECT
    cc.root_chat_id,
    COALESCE(rc.title, '') AS chat_title,
    cc.total_cost_micros,
    cc.step_count,
    cc.total_input_tokens,
    cc.total_output_tokens,
    cc.total_cache_read_tokens,
    cc.total_cache_creation_tokens
FROM chat_costs cc
LEFT JOIN chats rc ON rc.id = cc.root_chat_id
ORDER BY cc.total_cost_micros DESC;

-- name: GetChatCostPerUser :many
-- Deployment-wide per-user cost rollup within a date range.
WITH chat_cost_users AS (
    SELECT
        c.owner_id AS user_id,
        u.username,
        u.name,
        u.avatar_url,
        COALESCE(SUM(s.total_cost_micros), 0)::bigint AS total_cost_micros,
        COUNT(*)::bigint AS step_count,
        COUNT(DISTINCT COALESCE(c.root_chat_id, c.id))::bigint AS chat_count,
        COALESCE(SUM(s.input_tokens), 0)::bigint AS total_input_tokens,
        COALESCE(SUM(s.output_tokens), 0)::bigint AS total_output_tokens,
        COALESCE(SUM(s.cache_read_tokens), 0)::bigint AS total_cache_read_tokens,
        COALESCE(SUM(s.cache_creation_tokens), 0)::bigint AS total_cache_creation_tokens
    FROM
        chat_run_steps s
    JOIN
        chat_runs r ON r.id = s.chat_run_id
    JOIN
        chats c ON c.id = r.chat_id
    JOIN
        users u ON u.id = c.owner_id
    WHERE
        s.completed_at IS NOT NULL
        AND s.started_at >= @start_date::timestamptz
        AND s.started_at < @end_date::timestamptz
        AND (
            @username::text = ''
            OR u.username ILIKE '%' || @username::text || '%'
        )
    GROUP BY
        c.owner_id,
        u.username,
        u.name,
        u.avatar_url
)
SELECT
    user_id,
    username,
    name,
    avatar_url,
    total_cost_micros,
    step_count,
    chat_count,
    total_input_tokens,
    total_output_tokens,
    total_cache_read_tokens,
    total_cache_creation_tokens,
    COUNT(*) OVER()::bigint AS total_count
FROM
    chat_cost_users
ORDER BY
    total_cost_micros DESC,
    username ASC
LIMIT
    sqlc.arg('page_limit')::int
OFFSET
    sqlc.arg('page_offset')::int;
