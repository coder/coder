-- name: ArchiveChatByID :many
WITH chats AS (
    UPDATE chats
    SET archived = true, pin_order = 0, updated_at = NOW()
    WHERE id = @id::uuid OR root_chat_id = @id::uuid
    RETURNING *
)
SELECT *
FROM chats
ORDER BY (id = @id::uuid) DESC, created_at ASC, id ASC;

-- name: UnarchiveChatByID :many
-- Unarchives a chat (and its children). Stale file references are
-- handled automatically by FK cascades on chat_file_links: when
-- dbpurge deletes a chat_files row, the corresponding
-- chat_file_links rows are cascade-deleted by PostgreSQL.
WITH chats AS (
    UPDATE chats SET
        archived = false,
        updated_at = NOW()
    WHERE id = @id::uuid OR root_chat_id = @id::uuid
    RETURNING *
)
SELECT *
FROM chats
ORDER BY (id = @id::uuid) DESC, created_at ASC, id ASC;

-- name: PinChatByID :exec
WITH target_chat AS (
    SELECT
        id,
        owner_id
    FROM
        chats
    WHERE
        id = @id::uuid
),
-- Under READ COMMITTED, concurrent pin operations for the same
-- owner may momentarily produce duplicate pin_order values because
-- each CTE snapshot does not see the other's writes. The next
-- pin/unpin/reorder operation's ROW_NUMBER() self-heals the
-- sequence, so this is acceptable.
ranked AS (
    SELECT
        c.id,
        ROW_NUMBER() OVER (ORDER BY c.pin_order ASC, c.id ASC) :: integer AS next_pin_order
    FROM
        chats c
    JOIN
        target_chat ON c.owner_id = target_chat.owner_id
    WHERE
        c.pin_order > 0
        AND c.archived = FALSE
        AND c.id <> target_chat.id
),
updates AS (
    SELECT
        ranked.id,
        ranked.next_pin_order AS pin_order
    FROM
        ranked
    UNION ALL
    SELECT
        target_chat.id,
        COALESCE((
            SELECT
                MAX(ranked.next_pin_order)
            FROM
                ranked
        ), 0) + 1 AS pin_order
    FROM
        target_chat
)
UPDATE
    chats c
SET
    pin_order = updates.pin_order
FROM
    updates
WHERE
    c.id = updates.id;

-- name: UnpinChatByID :exec
WITH target_chat AS (
    SELECT
        id,
        owner_id
    FROM
        chats
    WHERE
        id = @id::uuid
),
ranked AS (
    SELECT
        c.id,
        ROW_NUMBER() OVER (ORDER BY c.pin_order ASC, c.id ASC) :: integer AS current_position
    FROM
        chats c
    JOIN
        target_chat ON c.owner_id = target_chat.owner_id
    WHERE
        c.pin_order > 0
        AND c.archived = FALSE
),
target AS (
    SELECT
        ranked.id,
        ranked.current_position
    FROM
        ranked
    WHERE
        ranked.id = @id::uuid
),
updates AS (
    SELECT
        ranked.id,
        CASE
            WHEN ranked.id = target.id THEN 0
            WHEN ranked.current_position > target.current_position THEN ranked.current_position - 1
            ELSE ranked.current_position
        END AS pin_order
    FROM
        ranked
    CROSS JOIN
        target
)
UPDATE
    chats c
SET
    pin_order = updates.pin_order
FROM
    updates
WHERE
    c.id = updates.id;

-- name: UpdateChatPinOrder :exec
WITH target_chat AS (
    SELECT
        id,
        owner_id
    FROM
        chats
    WHERE
        id = @id::uuid
),
ranked AS (
    SELECT
        c.id,
        ROW_NUMBER() OVER (ORDER BY c.pin_order ASC, c.id ASC) :: integer AS current_position,
        COUNT(*) OVER () :: integer AS pinned_count
    FROM
        chats c
    JOIN
        target_chat ON c.owner_id = target_chat.owner_id
    WHERE
        c.pin_order > 0
        AND c.archived = FALSE
),
target AS (
    SELECT
        ranked.id,
        ranked.current_position,
        LEAST(GREATEST(@pin_order::integer, 1), ranked.pinned_count) AS desired_position
    FROM
        ranked
    WHERE
        ranked.id = @id::uuid
),
updates AS (
    SELECT
        ranked.id,
        CASE
            WHEN ranked.id = target.id THEN target.desired_position
            WHEN target.desired_position < target.current_position
                AND ranked.current_position >= target.desired_position
                AND ranked.current_position < target.current_position THEN ranked.current_position + 1
            WHEN target.desired_position > target.current_position
                AND ranked.current_position > target.current_position
                AND ranked.current_position <= target.desired_position THEN ranked.current_position - 1
            ELSE ranked.current_position
        END AS pin_order
    FROM
        ranked
    CROSS JOIN
        target
)
UPDATE
    chats c
SET
    pin_order = updates.pin_order
FROM
    updates
WHERE
    c.id = updates.id;

-- name: SoftDeleteChatMessagesAfterID :exec
UPDATE
    chat_messages
SET
    deleted = true
WHERE
    chat_id = @chat_id::uuid
    AND id > @after_id::bigint;

-- name: SoftDeleteChatMessageByID :exec
UPDATE
    chat_messages
SET
    deleted = true
WHERE
    id = @id::bigint;

-- name: GetChatByID :one
SELECT
    *
FROM
    chats
WHERE
    id = @id::uuid;

-- name: GetChatMessageByID :one
SELECT
    *
FROM
    chat_messages
WHERE
    id = @id::bigint
    AND deleted = false;

-- name: GetChatMessagesByChatID :many
SELECT
    *
FROM
    chat_messages
WHERE
    chat_id = @chat_id::uuid
    AND id > @after_id::bigint
    AND visibility IN ('user', 'both')
    AND deleted = false
ORDER BY
    created_at ASC;

-- name: GetChatMessagesByChatIDAscPaginated :many
SELECT
    *
FROM
    chat_messages
WHERE
    chat_id = @chat_id::uuid
    AND id > @after_id::bigint
    AND visibility IN ('user', 'both')
    AND deleted = false
ORDER BY
    id ASC
LIMIT
    COALESCE(NULLIF(@limit_val::int, 0), 50);

-- name: GetChatMessagesByChatIDDescPaginated :many
SELECT
    *
FROM
    chat_messages
WHERE
    chat_id = @chat_id::uuid
    AND CASE
        WHEN @before_id::bigint > 0 THEN id < @before_id::bigint
        ELSE true
    END
    AND visibility IN ('user', 'both')
    AND deleted = false
ORDER BY
    id DESC
LIMIT
    COALESCE(NULLIF(@limit_val::int, 0), 50);

-- name: GetChatMessagesForPromptByChatID :many
WITH latest_compressed_summary AS (
    SELECT
        id
    FROM
        chat_messages
    WHERE
        chat_id = @chat_id::uuid
        AND compressed = TRUE
        AND deleted = false
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
    AND deleted = false
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

-- name: GetChats :many
SELECT
    sqlc.embed(chats),
    EXISTS (
        SELECT 1 FROM chat_messages cm
        WHERE cm.chat_id = chats.id
            AND cm.role = 'assistant'
            AND cm.deleted = false
            AND cm.id > COALESCE(chats.last_read_message_id, 0)
    ) AS has_unread
FROM
    chats
WHERE
    CASE
        WHEN @owner_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN chats.owner_id = @owner_id
        ELSE true
    END
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
    AND CASE
        WHEN sqlc.narg('label_filter')::jsonb IS NOT NULL THEN chats.labels @> sqlc.narg('label_filter')::jsonb
        ELSE true
    END
    -- Authorize Filter clause will be injected below in GetAuthorizedChats
    -- @authorize_filter
ORDER BY
    -- Deterministic and consistent ordering of all rows, even if they share
    -- a timestamp. This is to ensure consistent pagination.
    (updated_at, id) DESC OFFSET @offset_opt
LIMIT
    -- The chat list is unbounded and expected to grow large.
    -- Default to 50 to prevent accidental excessively large queries.
    COALESCE(NULLIF(@limit_opt :: int, 0), 50);

-- name: InsertChat :one
INSERT INTO chats (
    owner_id,
    workspace_id,
    build_id,
    agent_id,
    parent_chat_id,
    root_chat_id,
    last_model_config_id,
    title,
    mode,
    status,
    mcp_server_ids,
    labels,
    dynamic_tools
) VALUES (
    @owner_id::uuid,
    sqlc.narg('workspace_id')::uuid,
    sqlc.narg('build_id')::uuid,
    sqlc.narg('agent_id')::uuid,
    sqlc.narg('parent_chat_id')::uuid,
    sqlc.narg('root_chat_id')::uuid,
    @last_model_config_id::uuid,
    @title::text,
    sqlc.narg('mode')::chat_mode,
    @status::chat_status,
    COALESCE(@mcp_server_ids::uuid[], '{}'::uuid[]),
    COALESCE(sqlc.narg('labels')::jsonb, '{}'::jsonb),
    sqlc.narg('dynamic_tools')::jsonb
)
RETURNING
    *;

-- name: InsertChatMessages :many
WITH updated_chat AS (
    UPDATE
        chats
    SET
        last_model_config_id = (
            SELECT val
            FROM UNNEST(@model_config_id::uuid[])
                WITH ORDINALITY AS t(val, ord)
            WHERE val != '00000000-0000-0000-0000-000000000000'::uuid
            ORDER BY ord DESC
            LIMIT 1
        )
    WHERE
        id = @chat_id::uuid
        AND EXISTS (
            SELECT 1
            FROM UNNEST(@model_config_id::uuid[])
            WHERE unnest != '00000000-0000-0000-0000-000000000000'::uuid
        )
        AND chats.last_model_config_id IS DISTINCT FROM (
            SELECT val
            FROM UNNEST(@model_config_id::uuid[])
                WITH ORDINALITY AS t(val, ord)
            WHERE val != '00000000-0000-0000-0000-000000000000'::uuid
            ORDER BY ord DESC
            LIMIT 1
        )
)
INSERT INTO chat_messages (
    chat_id,
    created_by,
    model_config_id,
    role,
    content,
    content_version,
    visibility,
    input_tokens,
    output_tokens,
    total_tokens,
    reasoning_tokens,
    cache_creation_tokens,
    cache_read_tokens,
    context_limit,
    compressed,
    total_cost_micros,
    runtime_ms,
    provider_response_id
)
SELECT
    @chat_id::uuid,
    NULLIF(UNNEST(@created_by::uuid[]), '00000000-0000-0000-0000-000000000000'::uuid),
    NULLIF(UNNEST(@model_config_id::uuid[]), '00000000-0000-0000-0000-000000000000'::uuid),
    UNNEST(@role::chat_message_role[]),
    UNNEST(@content::text[])::jsonb,
    UNNEST(@content_version::smallint[]),
    UNNEST(@visibility::chat_message_visibility[]),
    NULLIF(UNNEST(@input_tokens::bigint[]), 0),
    NULLIF(UNNEST(@output_tokens::bigint[]), 0),
    NULLIF(UNNEST(@total_tokens::bigint[]), 0),
    NULLIF(UNNEST(@reasoning_tokens::bigint[]), 0),
    NULLIF(UNNEST(@cache_creation_tokens::bigint[]), 0),
    NULLIF(UNNEST(@cache_read_tokens::bigint[]), 0),
    NULLIF(UNNEST(@context_limit::bigint[]), 0),
    UNNEST(@compressed::boolean[]),
    NULLIF(UNNEST(@total_cost_micros::bigint[]), 0),
    NULLIF(UNNEST(@runtime_ms::bigint[]), 0),
    NULLIF(UNNEST(@provider_response_id::text[]), '')
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

-- name: UpdateChatLastModelConfigByID :one
UPDATE
    chats
SET
    -- NOTE: updated_at is intentionally NOT touched here to avoid changing list ordering.
    last_model_config_id = @last_model_config_id::uuid
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: UpdateChatLabelsByID :one
UPDATE
    chats
SET
    labels = @labels::jsonb,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: UpdateChatWorkspaceBinding :one
UPDATE chats SET
    workspace_id = sqlc.narg('workspace_id')::uuid,
    build_id = sqlc.narg('build_id')::uuid,
    agent_id = sqlc.narg('agent_id')::uuid,
    updated_at = NOW()
WHERE id = @id::uuid
RETURNING *;

-- name: UpdateChatBuildAgentBinding :one
UPDATE chats SET
    build_id = sqlc.narg('build_id')::uuid,
    agent_id = sqlc.narg('agent_id')::uuid,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING *;

-- name: UpdateChatLastInjectedContext :one
-- Updates the cached injected context parts (AGENTS.md +
-- skills) on the chat row. Called only when context changes
-- (first workspace attach or agent change). updated_at is
-- intentionally not touched to avoid reordering the chat list.
UPDATE chats SET
    last_injected_context = sqlc.narg('last_injected_context')::jsonb
WHERE
    id = @id::uuid
RETURNING *;

-- name: UpdateChatMCPServerIDs :one
UPDATE
    chats
SET
    mcp_server_ids = @mcp_server_ids::uuid[],
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: LinkChatFiles :one
-- LinkChatFiles inserts file associations into the chat_file_links
-- join table with deduplication (ON CONFLICT DO NOTHING). The INSERT
-- is conditional: it only proceeds when the total number of links
-- (existing + genuinely new) does not exceed max_file_links. Returns
-- the number of genuinely new file IDs that were NOT inserted due to
-- the cap. A return value of 0 means all files were linked (or were
-- already linked). A positive value means the cap blocked that many
-- new links.
WITH current AS (
    SELECT COUNT(*) AS cnt
    FROM chat_file_links
    WHERE chat_id = @chat_id::uuid
),
new_links AS (
    SELECT @chat_id::uuid AS chat_id, unnest(@file_ids::uuid[]) AS file_id
),
genuinely_new AS (
    SELECT nl.chat_id, nl.file_id
    FROM new_links nl
    WHERE NOT EXISTS (
        SELECT 1 FROM chat_file_links cfl
        WHERE cfl.chat_id = nl.chat_id AND cfl.file_id = nl.file_id
    )
),
inserted AS (
    INSERT INTO chat_file_links (chat_id, file_id)
    SELECT gn.chat_id, gn.file_id
    FROM genuinely_new gn, current c
    WHERE c.cnt + (SELECT COUNT(*) FROM genuinely_new) <= @max_file_links::int
    ON CONFLICT (chat_id, file_id) DO NOTHING
    RETURNING file_id
)
SELECT
    (SELECT COUNT(*)::int FROM genuinely_new) -
    (SELECT COUNT(*)::int FROM inserted) AS rejected_new_files;

-- name: AcquireChats :many
-- Acquires up to @num_chats pending chats for processing. Uses SKIP LOCKED
-- to prevent multiple replicas from acquiring the same chat.
UPDATE
    chats
SET
    status = 'running'::chat_status,
    started_at = @started_at::timestamptz,
    heartbeat_at = @started_at::timestamptz,
    updated_at = @started_at::timestamptz,
    worker_id = @worker_id::uuid
WHERE
    id = ANY(
        SELECT
            id
        FROM
            chats
        WHERE
            status = 'pending'::chat_status
        ORDER BY
            updated_at ASC
        FOR UPDATE
            SKIP LOCKED
        LIMIT
            @num_chats::int
    )
RETURNING
    *;

-- name: UpdateChatStatus :one
UPDATE
    chats
SET
    status = @status::chat_status,
    worker_id = sqlc.narg('worker_id')::uuid,
    started_at = sqlc.narg('started_at')::timestamptz,
    heartbeat_at = sqlc.narg('heartbeat_at')::timestamptz,
    last_error = sqlc.narg('last_error')::text,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: UpdateChatStatusPreserveUpdatedAt :one
UPDATE
    chats
SET
    status = @status::chat_status,
    worker_id = sqlc.narg('worker_id')::uuid,
    started_at = sqlc.narg('started_at')::timestamptz,
    heartbeat_at = sqlc.narg('heartbeat_at')::timestamptz,
    last_error = sqlc.narg('last_error')::text,
    updated_at = @updated_at::timestamptz
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: GetStaleChats :many
-- Find chats that appear stuck and need recovery. This covers:
--   1. Running chats whose heartbeat has expired (worker crash).
--   2. Chats awaiting client action (requires_action) past the
--      timeout threshold (client disappeared).
SELECT
    *
FROM
    chats
WHERE
    (status = 'running'::chat_status
        AND heartbeat_at < @stale_threshold::timestamptz)
    OR (status = 'requires_action'::chat_status
        AND updated_at < @stale_threshold::timestamptz);

-- name: UpdateChatHeartbeats :many
-- Bumps the heartbeat timestamp for the given set of chat IDs,
-- provided they are still running and owned by the specified
-- worker. Returns the IDs that were actually updated so the
-- caller can detect stolen or completed chats via set-difference.
UPDATE
    chats
SET
    heartbeat_at = @now::timestamptz
WHERE
    id = ANY(@ids::uuid[])
    AND worker_id = @worker_id::uuid
    AND status = 'running'::chat_status
RETURNING id;

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
    head_branch,
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
    sqlc.narg('head_branch')::text,
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
    head_branch = EXCLUDED.head_branch,
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
    AND deleted = false
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
        -- NOTE: updated_at is intentionally NOT touched here so
        -- the worker can read it as "when was this row last
        -- externally changed" (by MarkStale or a successful
        -- refresh).
        stale_at = NOW() + INTERVAL '5 minutes'
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
    -- NOTE: updated_at is intentionally NOT touched here so
    -- the worker can read it as "when was this row last
    -- externally changed" (by MarkStale or a successful
    -- refresh).
    stale_at = @stale_at::timestamptz
WHERE
    chat_id = @chat_id::uuid;

-- name: GetChatCostSummary :one
-- Aggregate cost summary for a single user within a date range.
-- Only counts assistant-role messages.
SELECT
    COALESCE(SUM(cm.total_cost_micros), 0)::bigint AS total_cost_micros,
    COUNT(*) FILTER (
        WHERE cm.total_cost_micros IS NOT NULL
    )::bigint AS priced_message_count,
    COUNT(*) FILTER (
        WHERE cm.total_cost_micros IS NULL
            AND (
                cm.input_tokens IS NOT NULL
                OR cm.output_tokens IS NOT NULL
                OR cm.reasoning_tokens IS NOT NULL
                OR cm.cache_creation_tokens IS NOT NULL
                OR cm.cache_read_tokens IS NOT NULL
            )
    )::bigint AS unpriced_message_count,
    COALESCE(SUM(cm.input_tokens), 0)::bigint AS total_input_tokens,
    COALESCE(SUM(cm.output_tokens), 0)::bigint AS total_output_tokens,
    COALESCE(SUM(cm.cache_read_tokens), 0)::bigint AS total_cache_read_tokens,
    COALESCE(SUM(cm.cache_creation_tokens), 0)::bigint AS total_cache_creation_tokens,
    COALESCE(SUM(cm.runtime_ms), 0)::bigint AS total_runtime_ms
FROM
    chat_messages cm
JOIN
    chats c ON c.id = cm.chat_id
WHERE
    c.owner_id = @owner_id::uuid
    AND cm.role = 'assistant'
    AND cm.created_at >= @start_date::timestamptz
    AND cm.created_at < @end_date::timestamptz;

-- name: GetChatCostPerModel :many
-- Per-model cost breakdown for a single user within a date range.
-- Only counts assistant-role messages that have a model_config_id.
SELECT
    cmc.id AS model_config_id,
    cmc.display_name,
    cmc.provider,
    cmc.model,
    COALESCE(SUM(cm.total_cost_micros), 0)::bigint AS total_cost_micros,
    COUNT(*) FILTER (
        WHERE cm.input_tokens IS NOT NULL
            OR cm.output_tokens IS NOT NULL
            OR cm.reasoning_tokens IS NOT NULL
            OR cm.cache_creation_tokens IS NOT NULL
            OR cm.cache_read_tokens IS NOT NULL
    )::bigint AS message_count,
    COALESCE(SUM(cm.input_tokens), 0)::bigint AS total_input_tokens,
    COALESCE(SUM(cm.output_tokens), 0)::bigint AS total_output_tokens,
    COALESCE(SUM(cm.cache_read_tokens), 0)::bigint AS total_cache_read_tokens,
    COALESCE(SUM(cm.cache_creation_tokens), 0)::bigint AS total_cache_creation_tokens,
    COALESCE(SUM(cm.runtime_ms), 0)::bigint AS total_runtime_ms
FROM
    chat_messages cm
JOIN
    chats c ON c.id = cm.chat_id
JOIN
    chat_model_configs cmc ON cmc.id = cm.model_config_id
WHERE
    c.owner_id = @owner_id::uuid
    AND cm.role = 'assistant'
    AND cm.created_at >= @start_date::timestamptz
    AND cm.created_at < @end_date::timestamptz
GROUP BY
    cmc.id, cmc.display_name, cmc.provider, cmc.model
ORDER BY
    total_cost_micros DESC;

-- name: GetChatCostPerChat :many
-- Per-root-chat cost breakdown for a single user within a date range.
-- Groups by root_chat_id so forked chats roll up under their root.
-- Only counts assistant-role messages.
WITH chat_costs AS (
    SELECT
        COALESCE(c.root_chat_id, c.id) AS root_chat_id,
        COALESCE(SUM(cm.total_cost_micros), 0)::bigint AS total_cost_micros,
        COUNT(*) FILTER (
            WHERE cm.input_tokens IS NOT NULL
                OR cm.output_tokens IS NOT NULL
                OR cm.reasoning_tokens IS NOT NULL
                OR cm.cache_creation_tokens IS NOT NULL
                OR cm.cache_read_tokens IS NOT NULL
        )::bigint AS message_count,
        COALESCE(SUM(cm.input_tokens), 0)::bigint AS total_input_tokens,
        COALESCE(SUM(cm.output_tokens), 0)::bigint AS total_output_tokens,
        COALESCE(SUM(cm.cache_read_tokens), 0)::bigint AS total_cache_read_tokens,
        COALESCE(SUM(cm.cache_creation_tokens), 0)::bigint AS total_cache_creation_tokens,
        COALESCE(SUM(cm.runtime_ms), 0)::bigint AS total_runtime_ms
    FROM chat_messages cm
    JOIN chats c ON c.id = cm.chat_id
    WHERE c.owner_id = @owner_id::uuid
      AND cm.role = 'assistant'
      AND cm.created_at >= @start_date::timestamptz
      AND cm.created_at < @end_date::timestamptz
    GROUP BY COALESCE(c.root_chat_id, c.id)
)
SELECT
    cc.root_chat_id,
    COALESCE(rc.title, '') AS chat_title,
    cc.total_cost_micros,
    cc.message_count,
    cc.total_input_tokens,
    cc.total_output_tokens,
    cc.total_cache_read_tokens,
    cc.total_cache_creation_tokens,
    cc.total_runtime_ms
FROM chat_costs cc
LEFT JOIN chats rc ON rc.id = cc.root_chat_id
ORDER BY cc.total_cost_micros DESC;

-- name: GetChatCostPerUser :many
-- Deployment-wide per-user cost rollup within a date range.
-- Only counts assistant-role messages.
WITH chat_cost_users AS (
    SELECT
        c.owner_id AS user_id,
        u.username,
        u.name,
        u.avatar_url,
        COALESCE(SUM(cm.total_cost_micros), 0)::bigint AS total_cost_micros,
        COUNT(*) FILTER (
            WHERE cm.input_tokens IS NOT NULL
                OR cm.output_tokens IS NOT NULL
                OR cm.reasoning_tokens IS NOT NULL
                OR cm.cache_creation_tokens IS NOT NULL
                OR cm.cache_read_tokens IS NOT NULL
        )::bigint AS message_count,
        COUNT(DISTINCT COALESCE(c.root_chat_id, c.id))::bigint AS chat_count,
        COALESCE(SUM(cm.input_tokens), 0)::bigint AS total_input_tokens,
        COALESCE(SUM(cm.output_tokens), 0)::bigint AS total_output_tokens,
        COALESCE(SUM(cm.cache_read_tokens), 0)::bigint AS total_cache_read_tokens,
        COALESCE(SUM(cm.cache_creation_tokens), 0)::bigint AS total_cache_creation_tokens,
        COALESCE(SUM(cm.runtime_ms), 0)::bigint AS total_runtime_ms
    FROM
        chat_messages cm
    JOIN
        chats c ON c.id = cm.chat_id
    JOIN
        users u ON u.id = c.owner_id
    WHERE
        cm.role = 'assistant'
        AND cm.created_at >= @start_date::timestamptz
        AND cm.created_at < @end_date::timestamptz
        AND (
            @username::text = ''
            OR u.username ILIKE '%' || @username::text || '%'
            OR u.name ILIKE '%' || @username::text || '%'
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
    message_count,
    chat_count,
    total_input_tokens,
    total_output_tokens,
    total_cache_read_tokens,
    total_cache_creation_tokens,
    total_runtime_ms,
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

-- name: GetChatUsageLimitConfig :one
SELECT * FROM chat_usage_limit_config WHERE singleton = TRUE LIMIT 1;

-- name: UpsertChatUsageLimitConfig :one
INSERT INTO chat_usage_limit_config (singleton, enabled, default_limit_micros, period, updated_at)
VALUES (TRUE, @enabled::boolean, @default_limit_micros::bigint, @period::text, NOW())
ON CONFLICT (singleton) DO UPDATE SET
    enabled = EXCLUDED.enabled,
    default_limit_micros = EXCLUDED.default_limit_micros,
    period = EXCLUDED.period,
    updated_at = NOW()
RETURNING *;

-- name: ListChatUsageLimitOverrides :many
SELECT u.id AS user_id, u.username, u.name, u.avatar_url,
       u.chat_spend_limit_micros AS spend_limit_micros
FROM users u
WHERE u.chat_spend_limit_micros IS NOT NULL
ORDER BY u.username ASC;

-- name: UpsertChatUsageLimitUserOverride :one
UPDATE users
SET chat_spend_limit_micros = @spend_limit_micros::bigint
WHERE id = @user_id::uuid
RETURNING id AS user_id, username, name, avatar_url, chat_spend_limit_micros AS spend_limit_micros;

-- name: DeleteChatUsageLimitUserOverride :exec
UPDATE users SET chat_spend_limit_micros = NULL WHERE id = @user_id::uuid;

-- name: GetChatUsageLimitUserOverride :one
SELECT id AS user_id, chat_spend_limit_micros AS spend_limit_micros
FROM users
WHERE id = @user_id::uuid AND chat_spend_limit_micros IS NOT NULL;

-- name: GetUserChatSpendInPeriod :one
SELECT COALESCE(SUM(cm.total_cost_micros), 0)::bigint AS total_spend_micros
FROM chat_messages cm
JOIN chats c ON c.id = cm.chat_id
WHERE c.owner_id = @user_id::uuid
  AND cm.created_at >= @start_time::timestamptz
  AND cm.created_at < @end_time::timestamptz
  AND cm.total_cost_micros IS NOT NULL;

-- name: CountEnabledModelsWithoutPricing :one
-- Counts enabled, non-deleted model configs that lack both input and
-- output pricing in their JSONB options.cost configuration.
SELECT COUNT(*)::bigint AS count
FROM chat_model_configs
WHERE enabled = TRUE
  AND deleted = FALSE
  AND (
    options->'cost' IS NULL
    OR options->'cost' = 'null'::jsonb
    OR (
      (options->'cost'->>'input_price_per_million_tokens' IS NULL)
      AND (options->'cost'->>'output_price_per_million_tokens' IS NULL)
    )
  );

-- name: ListChatUsageLimitGroupOverrides :many
SELECT
    g.id AS group_id,
    g.name AS group_name,
    g.display_name AS group_display_name,
    g.avatar_url AS group_avatar_url,
    g.chat_spend_limit_micros AS spend_limit_micros,
    (SELECT COUNT(*)
        FROM group_members_expanded gme
        WHERE gme.group_id = g.id
          AND gme.user_is_system = FALSE) AS member_count
FROM groups g
WHERE g.chat_spend_limit_micros IS NOT NULL
ORDER BY g.name ASC;

-- name: UpsertChatUsageLimitGroupOverride :one
UPDATE groups
SET chat_spend_limit_micros = @spend_limit_micros::bigint
WHERE id = @group_id::uuid
RETURNING id AS group_id, name, display_name, avatar_url, chat_spend_limit_micros AS spend_limit_micros;

-- name: DeleteChatUsageLimitGroupOverride :exec
UPDATE groups SET chat_spend_limit_micros = NULL WHERE id = @group_id::uuid;

-- name: GetChatUsageLimitGroupOverride :one
SELECT id AS group_id, chat_spend_limit_micros AS spend_limit_micros
FROM groups
WHERE id = @group_id::uuid AND chat_spend_limit_micros IS NOT NULL;

-- name: GetUserGroupSpendLimit :one
-- Returns the minimum (most restrictive) group limit for a user.
-- Returns -1 if the user has no group limits applied.
SELECT COALESCE(MIN(g.chat_spend_limit_micros), -1)::bigint AS limit_micros
FROM groups g
JOIN group_members_expanded gme ON gme.group_id = g.id
WHERE gme.user_id = @user_id::uuid
  AND g.chat_spend_limit_micros IS NOT NULL;

-- name: GetChatsByWorkspaceIDs :many
SELECT *
FROM chats
WHERE archived = false
  AND workspace_id = ANY(@ids::uuid[])
ORDER BY workspace_id, updated_at DESC;

-- name: ResolveUserChatSpendLimit :one
-- Resolves the effective spend limit for a user using the hierarchy:
-- 1. Individual user override (highest priority)
-- 2. Minimum group limit across all user's groups
-- 3. Global default from config
-- Returns -1 if limits are not enabled.
SELECT CASE
    -- If limits are disabled, return -1.
    WHEN NOT cfg.enabled THEN -1
    -- Individual override takes priority.
    WHEN u.chat_spend_limit_micros IS NOT NULL THEN u.chat_spend_limit_micros
    -- Group limit (minimum across all user's groups) is next.
    WHEN gl.limit_micros IS NOT NULL THEN gl.limit_micros
    -- Fall back to global default.
    ELSE cfg.default_limit_micros
END::bigint AS effective_limit_micros
FROM chat_usage_limit_config cfg
CROSS JOIN users u
LEFT JOIN LATERAL (
    SELECT MIN(g.chat_spend_limit_micros) AS limit_micros
    FROM groups g
    JOIN group_members_expanded gme ON gme.group_id = g.id
    WHERE gme.user_id = @user_id::uuid
      AND g.chat_spend_limit_micros IS NOT NULL
) gl ON TRUE
WHERE u.id = @user_id::uuid
LIMIT 1;

-- name: UpdateChatLastReadMessageID :exec
-- Updates the last read message ID for a chat. This is used to track
-- which messages the owner has seen, enabling unread indicators.
UPDATE chats
SET last_read_message_id = @last_read_message_id::bigint
WHERE id = @id::uuid;

-- name: DeleteOldChats :execrows
-- Deletes chats that have been archived for longer than the given
-- threshold. Active (non-archived) chats are never deleted.
-- Related chat_messages, chat_diff_statuses, and
-- chat_queued_messages are removed via ON DELETE CASCADE.
-- Parent/root references on child chats are SET NULL.
WITH deletable AS (
    SELECT id
    FROM chats
    WHERE archived = true
      AND updated_at < @before_time::timestamptz
    ORDER BY updated_at ASC
    LIMIT @limit_count
)
DELETE FROM chats
USING deletable
WHERE chats.id = deletable.id
  AND chats.archived = true;

-- name: GetChatsUpdatedAfter :many
-- Retrieves chats updated after the given timestamp for telemetry
-- snapshot collection. Uses updated_at so that long-running chats
-- still appear in each snapshot window while they are active.
SELECT
    id, owner_id, created_at, updated_at, status,
    (parent_chat_id IS NOT NULL)::bool AS has_parent,
    root_chat_id, workspace_id,
    mode, archived, last_model_config_id
FROM chats
WHERE updated_at > @updated_after;

-- name: GetChatMessageSummariesPerChat :many
-- Aggregates message-level metrics per chat for messages created
-- after the given timestamp. Uses message created_at so that
-- ongoing activity in long-running chats is captured each window.
SELECT
    cm.chat_id,
    COUNT(*)::bigint AS message_count,
    COUNT(*) FILTER (WHERE cm.role = 'user')::bigint AS user_message_count,
    COUNT(*) FILTER (WHERE cm.role = 'assistant')::bigint AS assistant_message_count,
    COUNT(*) FILTER (WHERE cm.role = 'tool')::bigint AS tool_message_count,
    COUNT(*) FILTER (WHERE cm.role = 'system')::bigint AS system_message_count,
    COALESCE(SUM(cm.input_tokens), 0)::bigint AS total_input_tokens,
    COALESCE(SUM(cm.output_tokens), 0)::bigint AS total_output_tokens,
    COALESCE(SUM(cm.reasoning_tokens), 0)::bigint AS total_reasoning_tokens,
    COALESCE(SUM(cm.cache_creation_tokens), 0)::bigint AS total_cache_creation_tokens,
    COALESCE(SUM(cm.cache_read_tokens), 0)::bigint AS total_cache_read_tokens,
    COALESCE(SUM(cm.total_cost_micros), 0)::bigint AS total_cost_micros,
    COALESCE(SUM(cm.runtime_ms), 0)::bigint AS total_runtime_ms,
    COUNT(DISTINCT cm.model_config_id)::bigint AS distinct_model_count,
    COUNT(*) FILTER (WHERE cm.compressed)::bigint AS compressed_message_count
FROM chat_messages cm
WHERE cm.created_at > @created_after
  AND cm.deleted = false
GROUP BY cm.chat_id;

-- name: GetChatModelConfigsForTelemetry :many
-- Returns all model configurations for telemetry snapshot collection.
SELECT id, provider, model, context_limit, enabled, is_default
FROM chat_model_configs
WHERE deleted = false;
