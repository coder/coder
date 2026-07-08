-- name: ArchiveChatByID :many
WITH updated_chats AS (
    UPDATE chats
    SET archived = true, pin_order = 0, updated_at = NOW()
    WHERE id = @id::uuid OR root_chat_id = @id::uuid
    RETURNING *
),
chats_expanded AS (
    SELECT
        updated_chats.id,
        updated_chats.owner_id,
        updated_chats.workspace_id,
        updated_chats.title,
        updated_chats.status,
        updated_chats.worker_id,
        updated_chats.started_at,
        updated_chats.heartbeat_at,
        updated_chats.created_at,
        updated_chats.updated_at,
        updated_chats.parent_chat_id,
        updated_chats.root_chat_id,
        updated_chats.last_model_config_id,
        updated_chats.archived,
        updated_chats.last_error,
        updated_chats.mode,
        updated_chats.mcp_server_ids,
        updated_chats.labels,
        updated_chats.build_id,
        updated_chats.agent_id,
        updated_chats.pin_order,
        updated_chats.last_read_message_id,
        updated_chats.dynamic_tools,
        updated_chats.organization_id,
        updated_chats.plan_mode,
        updated_chats.client_type,
        updated_chats.last_turn_summary,
        updated_chats.snapshot_version,
        updated_chats.history_version,
        updated_chats.queue_version,
        updated_chats.generation_attempt,
        updated_chats.retry_state,
        updated_chats.retry_state_version,
        updated_chats.runner_id,
        updated_chats.requires_action_deadline_at,
        COALESCE(root.user_acl, updated_chats.user_acl) AS user_acl,
        COALESCE(root.group_acl, updated_chats.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        updated_chats.context_aggregate_hash,
        updated_chats.context_dirty_since,
        updated_chats.context_dirty_resources,
        updated_chats.context_error
    FROM
        updated_chats
    LEFT JOIN chats root ON root.id = COALESCE(updated_chats.root_chat_id, updated_chats.parent_chat_id)
    JOIN visible_users owner ON owner.id = updated_chats.owner_id
)
SELECT *
FROM chats_expanded
ORDER BY (chats_expanded.id = @id::uuid) DESC, chats_expanded.created_at ASC, chats_expanded.id ASC;

-- name: UnarchiveChatByID :many
-- Unarchives a chat (and its children). Stale file references are
-- handled automatically by FK cascades on chat_file_links: when
-- dbpurge deletes a chat_files row, the corresponding
-- chat_file_links rows are cascade-deleted by PostgreSQL.
WITH updated_chats AS (
    UPDATE chats SET
        archived = false,
        updated_at = NOW()
    WHERE id = @id::uuid OR root_chat_id = @id::uuid
    RETURNING *
),
chats_expanded AS (
    SELECT
        updated_chats.id,
        updated_chats.owner_id,
        updated_chats.workspace_id,
        updated_chats.title,
        updated_chats.status,
        updated_chats.worker_id,
        updated_chats.started_at,
        updated_chats.heartbeat_at,
        updated_chats.created_at,
        updated_chats.updated_at,
        updated_chats.parent_chat_id,
        updated_chats.root_chat_id,
        updated_chats.last_model_config_id,
        updated_chats.archived,
        updated_chats.last_error,
        updated_chats.mode,
        updated_chats.mcp_server_ids,
        updated_chats.labels,
        updated_chats.build_id,
        updated_chats.agent_id,
        updated_chats.pin_order,
        updated_chats.last_read_message_id,
        updated_chats.dynamic_tools,
        updated_chats.organization_id,
        updated_chats.plan_mode,
        updated_chats.client_type,
        updated_chats.last_turn_summary,
        updated_chats.snapshot_version,
        updated_chats.history_version,
        updated_chats.queue_version,
        updated_chats.generation_attempt,
        updated_chats.retry_state,
        updated_chats.retry_state_version,
        updated_chats.runner_id,
        updated_chats.requires_action_deadline_at,
        COALESCE(root.user_acl, updated_chats.user_acl) AS user_acl,
        COALESCE(root.group_acl, updated_chats.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        updated_chats.context_aggregate_hash,
        updated_chats.context_dirty_since,
        updated_chats.context_dirty_resources,
        updated_chats.context_error
    FROM
        updated_chats
    LEFT JOIN chats root ON root.id = COALESCE(updated_chats.root_chat_id, updated_chats.parent_chat_id)
    JOIN visible_users owner ON owner.id = updated_chats.owner_id
)
SELECT *
FROM chats_expanded
ORDER BY (chats_expanded.id = @id::uuid) DESC, chats_expanded.created_at ASC, chats_expanded.id ASC;

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
SELECT *
FROM chats_expanded
WHERE id = @id::uuid;

-- name: GetChatFamilyIDsByRootID :many
-- Returns the chat IDs of every chat in a family (root + all children)
-- in deterministic order. The id parameter must be the root id; the
-- query does not walk up from a child.
SELECT id
FROM chats
WHERE id = @id::uuid OR root_chat_id = @id::uuid
ORDER BY (id = @id::uuid) DESC, created_at ASC, id ASC;

-- name: GetChatACLByID :one
SELECT
    user_acl AS users,
    group_acl AS groups
FROM
    chats
WHERE
    id = @id::uuid;

-- name: UpdateChatACLByID :exec
UPDATE
    chats
SET
    user_acl = @user_acl,
    group_acl = @group_acl
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

-- name: GetChatMessagesByRevisionForStream :many
SELECT
    *
FROM
    chat_messages
WHERE
    chat_id = @chat_id::uuid
    AND revision > @after_revision::bigint
    AND visibility IN ('user', 'both')
ORDER BY
    created_at ASC, id ASC;

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
    AND CASE
        WHEN @after_id::bigint > 0 THEN id > @after_id::bigint
        ELSE true
    END
    AND visibility IN ('user', 'both')
    AND deleted = false
ORDER BY
    id DESC
LIMIT
    COALESCE(NULLIF(@limit_val::int, 0), 50);

-- name: GetChatUserPromptsByChatID :many
-- Returns the concatenated text of each user-visible user prompt in a
-- chat, newest first. Used by the composer to populate the up/down
-- arrow prompt-history cycle. Non-text parts (tool calls, files,
-- attachments, ...) are excluded; messages whose text payload is
-- entirely whitespace are dropped so cycling never lands on a blank
-- entry. The jsonb_typeof guard skips legacy V0 rows whose content is
-- a scalar JSON string (predates migration 000434) so the lateral
-- jsonb_array_elements never raises "cannot extract elements from a
-- scalar". Backed by idx_chat_messages_user_prompts.
SELECT
    cm.id,
    string_agg(part->>'text', '' ORDER BY ordinality)::text AS text
FROM
    chat_messages cm,
    jsonb_array_elements(cm.content) WITH ORDINALITY AS t(part, ordinality)
WHERE
    cm.chat_id = @chat_id::uuid
    AND cm.role = 'user'
    AND cm.deleted = false
    AND cm.visibility IN ('user', 'both')
    AND jsonb_typeof(cm.content) = 'array'
    AND part->>'type' = 'text'
GROUP BY
    cm.id
HAVING
    string_agg(part->>'text', '') ~ '\S'
ORDER BY
    cm.id DESC
LIMIT
    COALESCE(NULLIF(@limit_val::int, 0), 500);

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
WITH cursor_chat AS (
    SELECT
        pin_order,
        updated_at,
        id
    FROM chats
    WHERE id = @after_id
)
SELECT
    sqlc.embed(chats_expanded),
    EXISTS (
        SELECT 1 FROM chat_messages cm
        WHERE cm.chat_id = chats_expanded.id
            AND cm.role = 'assistant'
            AND cm.deleted = false
            AND cm.id > COALESCE(chats_expanded.last_read_message_id, 0)
    ) AS has_unread
FROM
    chats_expanded
WHERE
    (
        (NOT @owned_only::boolean AND NOT @shared_only::boolean)
        OR (@owned_only::boolean AND chats_expanded.owner_id = @viewer_id::uuid)
        OR (
            @shared_only::boolean
            AND chats_expanded.owner_id != @viewer_id::uuid
            AND (
                chats_expanded.user_acl ? (@shared_with_user_id::uuid)::text
                OR chats_expanded.group_acl ?| @shared_with_group_ids::text[]
            )
        )
    )
    AND CASE
        WHEN sqlc.narg('archived') :: boolean IS NULL THEN true
        ELSE chats_expanded.archived = sqlc.narg('archived') :: boolean
    END
    AND CASE
        -- Cursor pagination: the last element on a page acts as the cursor.
        -- The 4-tuple matches the ORDER BY below. All columns sort DESC
        -- (pin_order is negated so lower values sort first in DESC order),
        -- which lets us use a single tuple < comparison.
        WHEN @after_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN (
            (CASE WHEN chats_expanded.pin_order > 0 THEN 1 ELSE 0 END, -chats_expanded.pin_order, chats_expanded.updated_at, chats_expanded.id) < (
                SELECT
                    CASE WHEN cursor_chat.pin_order > 0 THEN 1 ELSE 0 END,
                    -cursor_chat.pin_order,
                    cursor_chat.updated_at,
                    cursor_chat.id
                FROM
                    cursor_chat
            )
        )
        ELSE true
    END
    AND CASE
        WHEN sqlc.narg('label_filter')::jsonb IS NOT NULL THEN chats_expanded.labels @> sqlc.narg('label_filter')::jsonb
        ELSE true
    END
    -- Match chats whose linked diff URL (e.g. a pull request URL)
    -- equals the given value, case-insensitively. The URL may live on
    -- a delegated sub-agent's diff status, so we surface the root chat
    -- when any descendant matches.
    AND CASE
        WHEN sqlc.narg('diff_url')::text IS NOT NULL THEN EXISTS (
            SELECT 1
            FROM chat_diff_statuses cds
            JOIN chats c2 ON c2.id = cds.chat_id
            WHERE cds.url IS NOT NULL
              AND cds.url <> ''
              AND LOWER(cds.url) = LOWER(sqlc.narg('diff_url')::text)
              AND (c2.id = chats_expanded.id OR c2.root_chat_id = chats_expanded.id)
        )
        ELSE true
    END
    -- Filter by title substring (case-insensitive). Applied when the
    -- caller provides a non-empty title_query.
    AND CASE
        WHEN @title_query :: text != '' THEN chats_expanded.title ILIKE '%' || @title_query || '%'
        ELSE true
    END
    AND CASE
        WHEN sqlc.narg('has_unread')::boolean IS NOT NULL THEN (
            EXISTS (
                SELECT 1 FROM chat_messages cm
                WHERE cm.chat_id = chats_expanded.id
                    AND cm.role = 'assistant'
                    AND cm.deleted = false
                    AND cm.id > COALESCE(chats_expanded.last_read_message_id, 0)
            )
        ) = sqlc.narg('has_unread')::boolean
        ELSE true
    END
    -- Filter by pull request status. Unlike the diff_url filter above,
    -- this intentionally checks only the root chat's own diff status.
    -- Child chats share the same workspace and git branch as their
    -- parent, so gitsync populates identical PR state on both; traversing
    -- descendants would be redundant.
    AND CASE
        WHEN COALESCE(array_length(@pull_request_statuses::text[], 1), 0) > 0 THEN EXISTS (
            SELECT 1
            FROM chat_diff_statuses cds
            WHERE cds.chat_id = chats_expanded.id
                AND (
                    CASE
                        WHEN cds.pull_request_state = 'open' AND cds.pull_request_draft THEN 'draft'
                        WHEN cds.pull_request_state = 'open' THEN 'open'
                        ELSE cds.pull_request_state
                    END
                ) = ANY(@pull_request_statuses::text[])
        )
        ELSE true
    END
    -- Filter by PR number (exact match on chat's diff status).
    AND CASE
        WHEN @pr_number::int != 0 THEN EXISTS (
            SELECT 1
            FROM chat_diff_statuses cds
            WHERE cds.chat_id = chats_expanded.id
                AND cds.pr_number = @pr_number
        )
        ELSE true
    END
    -- Filter by repository (substring match on remote origin or PR URL).
    AND CASE
        WHEN @repo_query::text != '' THEN EXISTS (
            SELECT 1
            FROM chat_diff_statuses cds
            WHERE cds.chat_id = chats_expanded.id
                AND (
                    cds.git_remote_origin ILIKE '%' || @repo_query || '%'
                    OR cds.url ILIKE '%' || @repo_query || '%'
                )
        )
        ELSE true
    END
    -- Filter by pull request title (case-insensitive substring).
    AND CASE
        WHEN @pr_title_query::text != '' THEN EXISTS (
            SELECT 1
            FROM chat_diff_statuses cds
            WHERE cds.chat_id = chats_expanded.id
                AND cds.pull_request_title ILIKE '%' || @pr_title_query || '%'
        )
        ELSE true
    END
    -- Paginate over root chats only. Children are fetched
    -- separately via GetChildChatsByParentIDs and embedded under
    -- each parent. Other callers that need the full set should
    -- use a narrower query (e.g. GetChatsByWorkspaceIDs).
    AND chats_expanded.parent_chat_id IS NULL
    -- Authorize Filter clause will be injected below in GetAuthorizedChats
    -- @authorize_filter
ORDER BY
    -- Pinned chats (pin_order > 0) sort before unpinned ones. Within
    -- pinned chats, lower pin_order values come first. The negation
    -- trick (-pin_order) keeps all sort columns DESC so the cursor
    -- tuple < comparison works with uniform direction.
    CASE WHEN chats_expanded.pin_order > 0 THEN 1 ELSE 0 END DESC,
    -chats_expanded.pin_order DESC,
    chats_expanded.updated_at DESC,
    chats_expanded.id DESC
OFFSET @offset_opt
LIMIT
    -- The chat list is unbounded and expected to grow large.
    -- Default to 50 to prevent accidental excessively large queries.
    COALESCE(NULLIF(@limit_opt :: int, 0), 50);

-- name: GetChildChatsByParentIDs :many
-- Fetches child chats of the given parents, optionally filtered by
-- archive state (NULL = all, true/false = match). The archive
-- invariant (parent archived implies child archived) is enforced
-- at write time, not here.
SELECT
    sqlc.embed(chats_expanded),
    EXISTS (
        SELECT 1 FROM chat_messages cm
        WHERE cm.chat_id = chats_expanded.id
            AND cm.role = 'assistant'
            AND cm.deleted = false
            AND cm.id > COALESCE(chats_expanded.last_read_message_id, 0)
    ) AS has_unread
FROM
    chats_expanded
WHERE
    chats_expanded.parent_chat_id = ANY(@parent_ids :: uuid[])
    AND CASE
        WHEN sqlc.narg('archived') :: boolean IS NULL THEN true
        ELSE chats_expanded.archived = sqlc.narg('archived') :: boolean
    END
ORDER BY
    chats_expanded.created_at DESC,
    chats_expanded.id DESC;

-- name: InsertChat :one
WITH inserted_chat AS (
INSERT INTO chats (
    organization_id,
    owner_id,
    workspace_id,
    build_id,
    agent_id,
    parent_chat_id,
    root_chat_id,
    last_model_config_id,
    title,
    mode,
    plan_mode,
    status,
    mcp_server_ids,
    labels,
    dynamic_tools,
    client_type
) VALUES (
    @organization_id::uuid,
    @owner_id::uuid,
    sqlc.narg('workspace_id')::uuid,
    sqlc.narg('build_id')::uuid,
    sqlc.narg('agent_id')::uuid,
    sqlc.narg('parent_chat_id')::uuid,
    sqlc.narg('root_chat_id')::uuid,
    @last_model_config_id::uuid,
    @title::text,
    sqlc.narg('mode')::chat_mode,
    sqlc.narg('plan_mode')::chat_plan_mode,
    @status::chat_status,
    COALESCE(@mcp_server_ids::uuid[], '{}'::uuid[]),
    COALESCE(sqlc.narg('labels')::jsonb, '{}'::jsonb),
    sqlc.narg('dynamic_tools')::jsonb,
    @client_type::chat_client_type
)
RETURNING *
),
chats_expanded AS (
    SELECT
        inserted_chat.id,
        inserted_chat.owner_id,
        inserted_chat.workspace_id,
        inserted_chat.title,
        inserted_chat.status,
        inserted_chat.worker_id,
        inserted_chat.started_at,
        inserted_chat.heartbeat_at,
        inserted_chat.created_at,
        inserted_chat.updated_at,
        inserted_chat.parent_chat_id,
        inserted_chat.root_chat_id,
        inserted_chat.last_model_config_id,
        inserted_chat.archived,
        inserted_chat.last_error,
        inserted_chat.mode,
        inserted_chat.mcp_server_ids,
        inserted_chat.labels,
        inserted_chat.build_id,
        inserted_chat.agent_id,
        inserted_chat.pin_order,
        inserted_chat.last_read_message_id,
        inserted_chat.dynamic_tools,
        inserted_chat.organization_id,
        inserted_chat.plan_mode,
        inserted_chat.client_type,
        inserted_chat.last_turn_summary,
        inserted_chat.snapshot_version,
        inserted_chat.history_version,
        inserted_chat.queue_version,
        inserted_chat.generation_attempt,
        inserted_chat.retry_state,
        inserted_chat.retry_state_version,
        inserted_chat.runner_id,
        inserted_chat.requires_action_deadline_at,
        COALESCE(root.user_acl, inserted_chat.user_acl) AS user_acl,
        COALESCE(root.group_acl, inserted_chat.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        inserted_chat.context_aggregate_hash,
        inserted_chat.context_dirty_since,
        inserted_chat.context_dirty_resources,
        inserted_chat.context_error
    FROM
        inserted_chat
    LEFT JOIN chats root ON root.id = COALESCE(inserted_chat.root_chat_id, inserted_chat.parent_chat_id)
    JOIN visible_users owner ON owner.id = inserted_chat.owner_id
)
SELECT *
FROM chats_expanded;

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
    api_key_id,
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
    runtime_ms
)
SELECT
    @chat_id::uuid,
    NULLIF(UNNEST(@created_by::uuid[]), '00000000-0000-0000-0000-000000000000'::uuid),
    NULLIF(UNNEST(@api_key_id::text[]), ''),
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
    NULLIF(UNNEST(@runtime_ms::bigint[]), 0)
RETURNING
    *;

-- name: UpdateChatByID :one
WITH updated_chat AS (
UPDATE
    chats
SET
    title = @title::text,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING *
),
chats_expanded AS (
    SELECT
        updated_chat.id,
        updated_chat.owner_id,
        updated_chat.workspace_id,
        updated_chat.title,
        updated_chat.status,
        updated_chat.worker_id,
        updated_chat.started_at,
        updated_chat.heartbeat_at,
        updated_chat.created_at,
        updated_chat.updated_at,
        updated_chat.parent_chat_id,
        updated_chat.root_chat_id,
        updated_chat.last_model_config_id,
        updated_chat.archived,
        updated_chat.last_error,
        updated_chat.mode,
        updated_chat.mcp_server_ids,
        updated_chat.labels,
        updated_chat.build_id,
        updated_chat.agent_id,
        updated_chat.pin_order,
        updated_chat.last_read_message_id,
        updated_chat.dynamic_tools,
        updated_chat.organization_id,
        updated_chat.plan_mode,
        updated_chat.client_type,
        updated_chat.last_turn_summary,
        updated_chat.snapshot_version,
        updated_chat.history_version,
        updated_chat.queue_version,
        updated_chat.generation_attempt,
        updated_chat.retry_state,
        updated_chat.retry_state_version,
        updated_chat.runner_id,
        updated_chat.requires_action_deadline_at,
        COALESCE(root.user_acl, updated_chat.user_acl) AS user_acl,
        COALESCE(root.group_acl, updated_chat.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        updated_chat.context_aggregate_hash,
        updated_chat.context_dirty_since,
        updated_chat.context_dirty_resources,
        updated_chat.context_error
    FROM
        updated_chat
    LEFT JOIN chats root ON root.id = COALESCE(updated_chat.root_chat_id, updated_chat.parent_chat_id)
    JOIN visible_users owner ON owner.id = updated_chat.owner_id
)
SELECT *
FROM chats_expanded;

-- name: UpdateChatTitleByID :one
WITH updated_chat AS (
UPDATE
    chats
SET
    -- NOTE: updated_at is intentionally NOT touched here to avoid
    -- changing list ordering when a user renames an older chat
    -- out-of-band.
    title = @title::text
WHERE
    id = @id::uuid
RETURNING *
),
chats_expanded AS (
    SELECT
        updated_chat.id,
        updated_chat.owner_id,
        updated_chat.workspace_id,
        updated_chat.title,
        updated_chat.status,
        updated_chat.worker_id,
        updated_chat.started_at,
        updated_chat.heartbeat_at,
        updated_chat.created_at,
        updated_chat.updated_at,
        updated_chat.parent_chat_id,
        updated_chat.root_chat_id,
        updated_chat.last_model_config_id,
        updated_chat.archived,
        updated_chat.last_error,
        updated_chat.mode,
        updated_chat.mcp_server_ids,
        updated_chat.labels,
        updated_chat.build_id,
        updated_chat.agent_id,
        updated_chat.pin_order,
        updated_chat.last_read_message_id,
        updated_chat.dynamic_tools,
        updated_chat.organization_id,
        updated_chat.plan_mode,
        updated_chat.client_type,
        updated_chat.last_turn_summary,
        updated_chat.snapshot_version,
        updated_chat.history_version,
        updated_chat.queue_version,
        updated_chat.generation_attempt,
        updated_chat.retry_state,
        updated_chat.retry_state_version,
        updated_chat.runner_id,
        updated_chat.requires_action_deadline_at,
        COALESCE(root.user_acl, updated_chat.user_acl) AS user_acl,
        COALESCE(root.group_acl, updated_chat.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        updated_chat.context_aggregate_hash,
        updated_chat.context_dirty_since,
        updated_chat.context_dirty_resources,
        updated_chat.context_error
    FROM
        updated_chat
    LEFT JOIN chats root ON root.id = COALESCE(updated_chat.root_chat_id, updated_chat.parent_chat_id)
    JOIN visible_users owner ON owner.id = updated_chat.owner_id
)
SELECT *
FROM chats_expanded;

-- name: UpdateChatPlanModeByID :one
WITH updated_chat AS (
UPDATE
    chats
SET
    -- NOTE: updated_at is intentionally NOT touched here to avoid changing list ordering.
    plan_mode = sqlc.narg('plan_mode')::chat_plan_mode
WHERE
    id = @id::uuid
RETURNING *
),
chats_expanded AS (
    SELECT
        updated_chat.id,
        updated_chat.owner_id,
        updated_chat.workspace_id,
        updated_chat.title,
        updated_chat.status,
        updated_chat.worker_id,
        updated_chat.started_at,
        updated_chat.heartbeat_at,
        updated_chat.created_at,
        updated_chat.updated_at,
        updated_chat.parent_chat_id,
        updated_chat.root_chat_id,
        updated_chat.last_model_config_id,
        updated_chat.archived,
        updated_chat.last_error,
        updated_chat.mode,
        updated_chat.mcp_server_ids,
        updated_chat.labels,
        updated_chat.build_id,
        updated_chat.agent_id,
        updated_chat.pin_order,
        updated_chat.last_read_message_id,
        updated_chat.dynamic_tools,
        updated_chat.organization_id,
        updated_chat.plan_mode,
        updated_chat.client_type,
        updated_chat.last_turn_summary,
        updated_chat.snapshot_version,
        updated_chat.history_version,
        updated_chat.queue_version,
        updated_chat.generation_attempt,
        updated_chat.retry_state,
        updated_chat.retry_state_version,
        updated_chat.runner_id,
        updated_chat.requires_action_deadline_at,
        COALESCE(root.user_acl, updated_chat.user_acl) AS user_acl,
        COALESCE(root.group_acl, updated_chat.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        updated_chat.context_aggregate_hash,
        updated_chat.context_dirty_since,
        updated_chat.context_dirty_resources,
        updated_chat.context_error
    FROM
        updated_chat
    LEFT JOIN chats root ON root.id = COALESCE(updated_chat.root_chat_id, updated_chat.parent_chat_id)
    JOIN visible_users owner ON owner.id = updated_chat.owner_id
)
SELECT *
FROM chats_expanded;

-- name: UpdateChatLastModelConfigByID :one
WITH updated_chat AS (
UPDATE
    chats
SET
    -- NOTE: updated_at is intentionally NOT touched here to avoid changing list ordering.
    last_model_config_id = @last_model_config_id::uuid
WHERE
    id = @id::uuid
RETURNING *
),
chats_expanded AS (
    SELECT
        updated_chat.id,
        updated_chat.owner_id,
        updated_chat.workspace_id,
        updated_chat.title,
        updated_chat.status,
        updated_chat.worker_id,
        updated_chat.started_at,
        updated_chat.heartbeat_at,
        updated_chat.created_at,
        updated_chat.updated_at,
        updated_chat.parent_chat_id,
        updated_chat.root_chat_id,
        updated_chat.last_model_config_id,
        updated_chat.archived,
        updated_chat.last_error,
        updated_chat.mode,
        updated_chat.mcp_server_ids,
        updated_chat.labels,
        updated_chat.build_id,
        updated_chat.agent_id,
        updated_chat.pin_order,
        updated_chat.last_read_message_id,
        updated_chat.dynamic_tools,
        updated_chat.organization_id,
        updated_chat.plan_mode,
        updated_chat.client_type,
        updated_chat.last_turn_summary,
        updated_chat.snapshot_version,
        updated_chat.history_version,
        updated_chat.queue_version,
        updated_chat.generation_attempt,
        updated_chat.retry_state,
        updated_chat.retry_state_version,
        updated_chat.runner_id,
        updated_chat.requires_action_deadline_at,
        COALESCE(root.user_acl, updated_chat.user_acl) AS user_acl,
        COALESCE(root.group_acl, updated_chat.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        updated_chat.context_aggregate_hash,
        updated_chat.context_dirty_since,
        updated_chat.context_dirty_resources,
        updated_chat.context_error
    FROM
        updated_chat
    LEFT JOIN chats root ON root.id = COALESCE(updated_chat.root_chat_id, updated_chat.parent_chat_id)
    JOIN visible_users owner ON owner.id = updated_chat.owner_id
)
SELECT *
FROM chats_expanded;

-- name: UpdateChatLabelsByID :one
WITH updated_chat AS (
UPDATE
    chats
SET
    labels = @labels::jsonb,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING *
),
chats_expanded AS (
    SELECT
        updated_chat.id,
        updated_chat.owner_id,
        updated_chat.workspace_id,
        updated_chat.title,
        updated_chat.status,
        updated_chat.worker_id,
        updated_chat.started_at,
        updated_chat.heartbeat_at,
        updated_chat.created_at,
        updated_chat.updated_at,
        updated_chat.parent_chat_id,
        updated_chat.root_chat_id,
        updated_chat.last_model_config_id,
        updated_chat.archived,
        updated_chat.last_error,
        updated_chat.mode,
        updated_chat.mcp_server_ids,
        updated_chat.labels,
        updated_chat.build_id,
        updated_chat.agent_id,
        updated_chat.pin_order,
        updated_chat.last_read_message_id,
        updated_chat.dynamic_tools,
        updated_chat.organization_id,
        updated_chat.plan_mode,
        updated_chat.client_type,
        updated_chat.last_turn_summary,
        updated_chat.snapshot_version,
        updated_chat.history_version,
        updated_chat.queue_version,
        updated_chat.generation_attempt,
        updated_chat.retry_state,
        updated_chat.retry_state_version,
        updated_chat.runner_id,
        updated_chat.requires_action_deadline_at,
        COALESCE(root.user_acl, updated_chat.user_acl) AS user_acl,
        COALESCE(root.group_acl, updated_chat.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        updated_chat.context_aggregate_hash,
        updated_chat.context_dirty_since,
        updated_chat.context_dirty_resources,
        updated_chat.context_error
    FROM
        updated_chat
    LEFT JOIN chats root ON root.id = COALESCE(updated_chat.root_chat_id, updated_chat.parent_chat_id)
    JOIN visible_users owner ON owner.id = updated_chat.owner_id
)
SELECT *
FROM chats_expanded;

-- name: UpdateChatWorkspaceBinding :one
WITH updated_chat AS (
UPDATE chats SET
    workspace_id = sqlc.narg('workspace_id')::uuid,
    build_id = sqlc.narg('build_id')::uuid,
    agent_id = sqlc.narg('agent_id')::uuid,
    updated_at = NOW()
WHERE id = @id::uuid
RETURNING *
),
chats_expanded AS (
    SELECT
        updated_chat.id,
        updated_chat.owner_id,
        updated_chat.workspace_id,
        updated_chat.title,
        updated_chat.status,
        updated_chat.worker_id,
        updated_chat.started_at,
        updated_chat.heartbeat_at,
        updated_chat.created_at,
        updated_chat.updated_at,
        updated_chat.parent_chat_id,
        updated_chat.root_chat_id,
        updated_chat.last_model_config_id,
        updated_chat.archived,
        updated_chat.last_error,
        updated_chat.mode,
        updated_chat.mcp_server_ids,
        updated_chat.labels,
        updated_chat.build_id,
        updated_chat.agent_id,
        updated_chat.pin_order,
        updated_chat.last_read_message_id,
        updated_chat.dynamic_tools,
        updated_chat.organization_id,
        updated_chat.plan_mode,
        updated_chat.client_type,
        updated_chat.last_turn_summary,
        updated_chat.snapshot_version,
        updated_chat.history_version,
        updated_chat.queue_version,
        updated_chat.generation_attempt,
        updated_chat.retry_state,
        updated_chat.retry_state_version,
        updated_chat.runner_id,
        updated_chat.requires_action_deadline_at,
        COALESCE(root.user_acl, updated_chat.user_acl) AS user_acl,
        COALESCE(root.group_acl, updated_chat.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        updated_chat.context_aggregate_hash,
        updated_chat.context_dirty_since,
        updated_chat.context_dirty_resources,
        updated_chat.context_error
    FROM
        updated_chat
    LEFT JOIN chats root ON root.id = COALESCE(updated_chat.root_chat_id, updated_chat.parent_chat_id)
    JOIN visible_users owner ON owner.id = updated_chat.owner_id
)
SELECT *
FROM chats_expanded;

-- name: UpdateChatBuildAgentBinding :one
WITH updated_chat AS (
UPDATE chats SET
    build_id = sqlc.narg('build_id')::uuid,
    agent_id = sqlc.narg('agent_id')::uuid,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING *
),
chats_expanded AS (
    SELECT
        updated_chat.id,
        updated_chat.owner_id,
        updated_chat.workspace_id,
        updated_chat.title,
        updated_chat.status,
        updated_chat.worker_id,
        updated_chat.started_at,
        updated_chat.heartbeat_at,
        updated_chat.created_at,
        updated_chat.updated_at,
        updated_chat.parent_chat_id,
        updated_chat.root_chat_id,
        updated_chat.last_model_config_id,
        updated_chat.archived,
        updated_chat.last_error,
        updated_chat.mode,
        updated_chat.mcp_server_ids,
        updated_chat.labels,
        updated_chat.build_id,
        updated_chat.agent_id,
        updated_chat.pin_order,
        updated_chat.last_read_message_id,
        updated_chat.dynamic_tools,
        updated_chat.organization_id,
        updated_chat.plan_mode,
        updated_chat.client_type,
        updated_chat.last_turn_summary,
        updated_chat.snapshot_version,
        updated_chat.history_version,
        updated_chat.queue_version,
        updated_chat.generation_attempt,
        updated_chat.retry_state,
        updated_chat.retry_state_version,
        updated_chat.runner_id,
        updated_chat.requires_action_deadline_at,
        COALESCE(root.user_acl, updated_chat.user_acl) AS user_acl,
        COALESCE(root.group_acl, updated_chat.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        updated_chat.context_aggregate_hash,
        updated_chat.context_dirty_since,
        updated_chat.context_dirty_resources,
        updated_chat.context_error
    FROM
        updated_chat
    LEFT JOIN chats root ON root.id = COALESCE(updated_chat.root_chat_id, updated_chat.parent_chat_id)
    JOIN visible_users owner ON owner.id = updated_chat.owner_id
)
SELECT *
FROM chats_expanded;

-- name: UpdateChatLastTurnSummary :execrows
-- Updates the cached last completed turn summary for sidebar display.
-- Empty or whitespace-only summaries are stored as NULL here so direct
-- query callers cannot accidentally persist blank sidebar text.
-- This intentionally preserves updated_at. The staleness guard uses
-- history_version so worker lifecycle transitions that do not change the
-- active message history cannot reject final turn summary writes.
-- Two summary workers using the same freshness marker are last-write-wins.
UPDATE chats
SET
    last_turn_summary = NULLIF(REGEXP_REPLACE(
        sqlc.narg('last_turn_summary')::text, '^[[:space:]]+|[[:space:]]+$', '', 'g'
    ), '')
WHERE
    id = @id::uuid
    AND history_version = @expected_history_version::bigint;

-- name: UpdateChatMCPServerIDs :one
WITH updated_chat AS (
UPDATE
    chats
SET
    mcp_server_ids = @mcp_server_ids::uuid[],
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING *
),
chats_expanded AS (
    SELECT
        updated_chat.id,
        updated_chat.owner_id,
        updated_chat.workspace_id,
        updated_chat.title,
        updated_chat.status,
        updated_chat.worker_id,
        updated_chat.started_at,
        updated_chat.heartbeat_at,
        updated_chat.created_at,
        updated_chat.updated_at,
        updated_chat.parent_chat_id,
        updated_chat.root_chat_id,
        updated_chat.last_model_config_id,
        updated_chat.archived,
        updated_chat.last_error,
        updated_chat.mode,
        updated_chat.mcp_server_ids,
        updated_chat.labels,
        updated_chat.build_id,
        updated_chat.agent_id,
        updated_chat.pin_order,
        updated_chat.last_read_message_id,
        updated_chat.dynamic_tools,
        updated_chat.organization_id,
        updated_chat.plan_mode,
        updated_chat.client_type,
        updated_chat.last_turn_summary,
        updated_chat.snapshot_version,
        updated_chat.history_version,
        updated_chat.queue_version,
        updated_chat.generation_attempt,
        updated_chat.retry_state,
        updated_chat.retry_state_version,
        updated_chat.runner_id,
        updated_chat.requires_action_deadline_at,
        COALESCE(root.user_acl, updated_chat.user_acl) AS user_acl,
        COALESCE(root.group_acl, updated_chat.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        updated_chat.context_aggregate_hash,
        updated_chat.context_dirty_since,
        updated_chat.context_dirty_resources,
        updated_chat.context_error
    FROM
        updated_chat
    LEFT JOIN chats root ON root.id = COALESCE(updated_chat.root_chat_id, updated_chat.parent_chat_id)
    JOIN visible_users owner ON owner.id = updated_chat.owner_id
)
SELECT *
FROM chats_expanded;

-- name: SetChatContextSnapshot :exec
-- Pins a single chat to the supplied context snapshot hash and error
-- and clears any dirty marker. Used by chat-create hydration and the
-- refresh endpoint. Does not bump updated_at: context pinning is
-- background state and must not reorder chat lists.
UPDATE chats
SET
    context_aggregate_hash = @aggregate_hash,
    context_error = @context_error,
    context_dirty_since = NULL
WHERE id = @id::uuid;

-- name: HydrateAgentChatsContext :exec
-- Stamps the pinned hash and error on every not-yet-hydrated chat for
-- an agent (context_aggregate_hash IS NULL) and copies the agent's
-- current context resources onto those chats in the same statement, so
-- a chat's pinned hash and pinned bodies are always written together.
-- Runs as a side effect of an agent push and of chat-create hydration,
-- so chats created before the agent was ready pick up the snapshot
-- without a dirty event. The ON CONFLICT upsert is defensive: a
-- not-yet-hydrated chat has no pinned rows, so it normally inserts.
-- Does not bump chats.updated_at; the resource upsert's ON CONFLICT branch
-- sets chat_context_resources.updated_at on the rows it rewrites.
WITH hydrated AS (
    UPDATE chats
    SET
        context_aggregate_hash = @aggregate_hash,
        context_error = @context_error
    WHERE agent_id = @agent_id::uuid
        AND archived = false
        AND context_aggregate_hash IS NULL
    RETURNING id
)
INSERT INTO chat_context_resources (
    chat_id, source, body_kind, body, content_hash, size_bytes, status, error, source_path
)
SELECT
    hydrated.id, r.source, r.body_kind, r.body, r.content_hash,
    r.size_bytes, r.status, r.error, r.source_path
FROM hydrated
CROSS JOIN workspace_agent_context_resources r
WHERE r.workspace_agent_id = @agent_id::uuid
ON CONFLICT (chat_id, source) DO UPDATE SET
    body_kind = EXCLUDED.body_kind,
    body = EXCLUDED.body,
    content_hash = EXCLUDED.content_hash,
    size_bytes = EXCLUDED.size_bytes,
    status = EXCLUDED.status,
    error = EXCLUDED.error,
    source_path = EXCLUDED.source_path,
    updated_at = now();

-- name: MarkChatsContextDirtyByAgent :many
-- Flips active, already-hydrated chats for an agent to dirty when the
-- agent's latest snapshot hash differs from the chat's pinned hash. The
-- pinned hash is intentionally left untouched; the refresh endpoint
-- re-pins it. Returns the chats that transitioned so the caller can
-- emit watch events after the transaction commits.
UPDATE chats
SET context_dirty_since = @dirty_since
WHERE agent_id = @agent_id::uuid
    AND archived = false
    AND status IN ('waiting', 'running', 'paused', 'pending', 'requires_action')
    AND context_aggregate_hash IS NOT NULL
    AND context_aggregate_hash IS DISTINCT FROM @aggregate_hash
    AND context_dirty_since IS NULL
RETURNING id, owner_id;

-- name: InsertAgentContextResourcesIntoChat :exec
-- Copies an agent's current context resources onto a single chat. Pair
-- with DeleteChatContextResourcesByChatID (clear-then-copy, in a
-- transaction) to re-pin a chat to its agent's latest snapshot from the
-- refresh endpoint and on agent rebinding.
INSERT INTO chat_context_resources (
    chat_id, source, body_kind, body, content_hash, size_bytes, status, error, source_path
)
SELECT
    @chat_id::uuid, r.source, r.body_kind, r.body, r.content_hash,
    r.size_bytes, r.status, r.error, r.source_path
FROM workspace_agent_context_resources r
WHERE r.workspace_agent_id = @agent_id::uuid;

-- name: DeleteChatContextResourcesByChatID :exec
-- Clears a chat's pinned context resources. Used as the first half of a
-- clear-then-copy re-pin, and on its own when the chat's current agent
-- has no snapshot.
DELETE FROM chat_context_resources
WHERE chat_id = @chat_id::uuid;

-- name: ListChatContextResourcesByChatID :many
-- Lists a chat's pinned context resources, ordered deterministically by
-- source.
SELECT * FROM chat_context_resources
WHERE chat_id = @chat_id::uuid
ORDER BY source ASC;

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
WITH acquired_chats AS (
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
            AND archived = false
        ORDER BY
            updated_at ASC
        FOR UPDATE
            SKIP LOCKED
        LIMIT
            @num_chats::int
    )
RETURNING *
),
chats_expanded AS (
    SELECT
        acquired_chats.id,
        acquired_chats.owner_id,
        acquired_chats.workspace_id,
        acquired_chats.title,
        acquired_chats.status,
        acquired_chats.worker_id,
        acquired_chats.started_at,
        acquired_chats.heartbeat_at,
        acquired_chats.created_at,
        acquired_chats.updated_at,
        acquired_chats.parent_chat_id,
        acquired_chats.root_chat_id,
        acquired_chats.last_model_config_id,
        acquired_chats.archived,
        acquired_chats.last_error,
        acquired_chats.mode,
        acquired_chats.mcp_server_ids,
        acquired_chats.labels,
        acquired_chats.build_id,
        acquired_chats.agent_id,
        acquired_chats.pin_order,
        acquired_chats.last_read_message_id,
        acquired_chats.dynamic_tools,
        acquired_chats.organization_id,
        acquired_chats.plan_mode,
        acquired_chats.client_type,
        acquired_chats.last_turn_summary,
        acquired_chats.snapshot_version,
        acquired_chats.history_version,
        acquired_chats.queue_version,
        acquired_chats.generation_attempt,
        acquired_chats.retry_state,
        acquired_chats.retry_state_version,
        acquired_chats.runner_id,
        acquired_chats.requires_action_deadline_at,
        COALESCE(root.user_acl, acquired_chats.user_acl) AS user_acl,
        COALESCE(root.group_acl, acquired_chats.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        acquired_chats.context_aggregate_hash,
        acquired_chats.context_dirty_since,
        acquired_chats.context_dirty_resources,
        acquired_chats.context_error
    FROM
        acquired_chats
    LEFT JOIN chats root ON root.id = COALESCE(acquired_chats.root_chat_id, acquired_chats.parent_chat_id)
    JOIN visible_users owner ON owner.id = acquired_chats.owner_id
)
SELECT *
FROM chats_expanded;

-- name: UpdateChatStatus :one
WITH updated_chat AS (
UPDATE
    chats
SET
    status = @status::chat_status,
    worker_id = sqlc.narg('worker_id')::uuid,
    started_at = sqlc.narg('started_at')::timestamptz,
    heartbeat_at = sqlc.narg('heartbeat_at')::timestamptz,
    last_error = sqlc.narg('last_error')::jsonb,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING *
),
chats_expanded AS (
    SELECT
        updated_chat.id,
        updated_chat.owner_id,
        updated_chat.workspace_id,
        updated_chat.title,
        updated_chat.status,
        updated_chat.worker_id,
        updated_chat.started_at,
        updated_chat.heartbeat_at,
        updated_chat.created_at,
        updated_chat.updated_at,
        updated_chat.parent_chat_id,
        updated_chat.root_chat_id,
        updated_chat.last_model_config_id,
        updated_chat.archived,
        updated_chat.last_error,
        updated_chat.mode,
        updated_chat.mcp_server_ids,
        updated_chat.labels,
        updated_chat.build_id,
        updated_chat.agent_id,
        updated_chat.pin_order,
        updated_chat.last_read_message_id,
        updated_chat.dynamic_tools,
        updated_chat.organization_id,
        updated_chat.plan_mode,
        updated_chat.client_type,
        updated_chat.last_turn_summary,
        updated_chat.snapshot_version,
        updated_chat.history_version,
        updated_chat.queue_version,
        updated_chat.generation_attempt,
        updated_chat.retry_state,
        updated_chat.retry_state_version,
        updated_chat.runner_id,
        updated_chat.requires_action_deadline_at,
        COALESCE(root.user_acl, updated_chat.user_acl) AS user_acl,
        COALESCE(root.group_acl, updated_chat.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        updated_chat.context_aggregate_hash,
        updated_chat.context_dirty_since,
        updated_chat.context_dirty_resources,
        updated_chat.context_error
    FROM
        updated_chat
    LEFT JOIN chats root ON root.id = COALESCE(updated_chat.root_chat_id, updated_chat.parent_chat_id)
    JOIN visible_users owner ON owner.id = updated_chat.owner_id
)
SELECT *
FROM chats_expanded;

-- name: GetStaleChats :many
-- Find chats that appear stuck and need recovery:
--   1. Running chats whose heartbeat has expired (worker crash).
--   2. requires_action chats past the timeout threshold (client
--      disappeared).
--   3. Waiting chats with a non-empty queue and stale updated_at
--      (deferred-promote stranding when the worker dies before its
--      post-cancel cleanup runs).
SELECT
    *
FROM
    chats_expanded
WHERE
    (status = 'running'::chat_status
        AND heartbeat_at < @stale_threshold::timestamptz)
    OR (status = 'requires_action'::chat_status
        AND updated_at < @stale_threshold::timestamptz)
    OR (status = 'waiting'::chat_status
        AND updated_at < @stale_threshold::timestamptz
        AND EXISTS (
            SELECT 1 FROM chat_queued_messages cqm
            WHERE cqm.chat_id = chats_expanded.id
        ));

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
-- Legacy queue insertion path. When no caller-supplied creator exists,
-- preserve the created_by invariant by attributing the queued row to the
-- chat owner.
INSERT INTO chat_queued_messages (chat_id, content, model_config_id, api_key_id, created_by)
SELECT
    @chat_id::uuid,
    @content::jsonb,
    sqlc.narg('model_config_id')::uuid,
    sqlc.narg('api_key_id')::text,
    chats.owner_id
FROM chats
WHERE chats.id = @chat_id::uuid
RETURNING *;

-- name: GetChatQueuedMessages :many
SELECT * FROM chat_queued_messages
WHERE chat_id = @chat_id
ORDER BY created_at ASC, id ASC;

-- name: DeleteChatQueuedMessage :exec
DELETE FROM chat_queued_messages WHERE id = @id AND chat_id = @chat_id;

-- name: DeleteAllChatQueuedMessages :exec
DELETE FROM chat_queued_messages WHERE chat_id = @chat_id;

-- name: PopNextQueuedMessage :one
DELETE FROM chat_queued_messages
WHERE id = (
    SELECT cqm.id FROM chat_queued_messages cqm
    WHERE cqm.chat_id = @chat_id
    ORDER BY cqm.created_at ASC, cqm.id ASC
    LIMIT 1
)
RETURNING *;

-- name: ReorderChatQueuedMessageToFront :execrows
-- Mutates only created_at on the target row; ids are unchanged so
-- consumers can keep tracking queued messages by id.
UPDATE chat_queued_messages AS target
SET created_at = (
    SELECT MIN(inner_cqm.created_at) - INTERVAL '1 microsecond'
    FROM chat_queued_messages AS inner_cqm
    WHERE inner_cqm.chat_id = @chat_id
)
WHERE target.id = @target_id AND target.chat_id = @chat_id;

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
WITH locked_chat AS (
    SELECT *
    FROM chats
    WHERE id = @id::uuid
    FOR UPDATE
),
chats_expanded AS (
    SELECT
        locked_chat.id,
        locked_chat.owner_id,
        locked_chat.workspace_id,
        locked_chat.title,
        locked_chat.status,
        locked_chat.worker_id,
        locked_chat.started_at,
        locked_chat.heartbeat_at,
        locked_chat.created_at,
        locked_chat.updated_at,
        locked_chat.parent_chat_id,
        locked_chat.root_chat_id,
        locked_chat.last_model_config_id,
        locked_chat.archived,
        locked_chat.last_error,
        locked_chat.mode,
        locked_chat.mcp_server_ids,
        locked_chat.labels,
        locked_chat.build_id,
        locked_chat.agent_id,
        locked_chat.pin_order,
        locked_chat.last_read_message_id,
        locked_chat.dynamic_tools,
        locked_chat.organization_id,
        locked_chat.plan_mode,
        locked_chat.client_type,
        locked_chat.last_turn_summary,
        locked_chat.snapshot_version,
        locked_chat.history_version,
        locked_chat.queue_version,
        locked_chat.generation_attempt,
        locked_chat.retry_state,
        locked_chat.retry_state_version,
        locked_chat.runner_id,
        locked_chat.requires_action_deadline_at,
        COALESCE(root.user_acl, locked_chat.user_acl) AS user_acl,
        COALESCE(root.group_acl, locked_chat.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        locked_chat.context_aggregate_hash,
        locked_chat.context_dirty_since,
        locked_chat.context_dirty_resources,
        locked_chat.context_error
    FROM
        locked_chat
    LEFT JOIN chats root ON root.id = COALESCE(locked_chat.root_chat_id, locked_chat.parent_chat_id)
    JOIN visible_users owner ON owner.id = locked_chat.owner_id
)
SELECT *
FROM chats_expanded;

-- name: GetChatByIDForShare :one
WITH shared_chat AS (
    SELECT *
    FROM chats
    WHERE id = @id::uuid
    FOR SHARE
),
chats_expanded AS (
    SELECT
        shared_chat.id,
        shared_chat.owner_id,
        shared_chat.workspace_id,
        shared_chat.title,
        shared_chat.status,
        shared_chat.worker_id,
        shared_chat.started_at,
        shared_chat.heartbeat_at,
        shared_chat.created_at,
        shared_chat.updated_at,
        shared_chat.parent_chat_id,
        shared_chat.root_chat_id,
        shared_chat.last_model_config_id,
        shared_chat.archived,
        shared_chat.last_error,
        shared_chat.mode,
        shared_chat.mcp_server_ids,
        shared_chat.labels,
        shared_chat.build_id,
        shared_chat.agent_id,
        shared_chat.pin_order,
        shared_chat.last_read_message_id,
        shared_chat.dynamic_tools,
        shared_chat.organization_id,
        shared_chat.plan_mode,
        shared_chat.client_type,
        shared_chat.last_turn_summary,
        shared_chat.snapshot_version,
        shared_chat.history_version,
        shared_chat.queue_version,
        shared_chat.generation_attempt,
        shared_chat.retry_state,
        shared_chat.retry_state_version,
        shared_chat.runner_id,
        shared_chat.requires_action_deadline_at,
        COALESCE(root.user_acl, shared_chat.user_acl) AS user_acl,
        COALESCE(root.group_acl, shared_chat.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        shared_chat.context_aggregate_hash,
        shared_chat.context_dirty_since,
        shared_chat.context_dirty_resources,
        shared_chat.context_error
    FROM
        shared_chat
    LEFT JOIN chats root ON root.id = COALESCE(shared_chat.root_chat_id, shared_chat.parent_chat_id)
    JOIN visible_users owner ON owner.id = shared_chat.owner_id
)
SELECT *
FROM chats_expanded;

-- name: GetChatsByChatFileID :many
SELECT
    *
FROM
    chats_expanded
WHERE
    id IN (
        SELECT chat_id
        FROM chat_file_links
        WHERE file_id = @file_id::uuid
    )
    -- Authorize Filter clause will be injected below in GetAuthorizedChatsByChatFileID.
    -- @authorize_filter
;

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

-- name: GetChatDiffStatusSummary :one
-- Returns aggregate PR counts across all agent chats for telemetry.
-- Deduplicates by PR URL so forked chats referencing the same pull
-- request are counted once (using the most recently refreshed state).
-- Total is derived from the three recognized state buckets and
-- always equals open + merged + closed; other non-NULL states are
-- intentionally excluded from these aggregates.
WITH deduped AS (
    SELECT DISTINCT ON (COALESCE(NULLIF(cds.url, ''), c.id::text))
        cds.pull_request_state
    FROM chat_diff_statuses cds
    JOIN chats c ON c.id = cds.chat_id
    WHERE cds.pull_request_state IN ('open', 'merged', 'closed')
    ORDER BY COALESCE(NULLIF(cds.url, ''), c.id::text), cds.updated_at DESC, c.id DESC
)
SELECT
    COUNT(*)::bigint AS total,
    COUNT(*) FILTER (WHERE pull_request_state = 'open')::bigint AS open,
    COUNT(*) FILTER (WHERE pull_request_state = 'merged')::bigint AS merged,
    COUNT(*) FILTER (WHERE pull_request_state = 'closed')::bigint AS closed
FROM deduped;

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
    COALESCE(ap.type::text, '')::text AS provider,
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
LEFT JOIN
    ai_providers ap ON ap.id = cmc.ai_provider_id
WHERE
    c.owner_id = @owner_id::uuid
    AND cm.role = 'assistant'
    AND cm.created_at >= @start_date::timestamptz
    AND cm.created_at < @end_date::timestamptz
GROUP BY
    cmc.id, cmc.display_name, ap.type, cmc.model
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
-- Returns the total spend for a user in the given period.
-- When organization_id is NULL, spend across all organizations is
-- returned (global behavior). Otherwise only spend within the
-- specified organization is included.
SELECT COALESCE(SUM(cm.total_cost_micros), 0)::bigint AS total_spend_micros
FROM chat_messages cm
JOIN chats c ON c.id = cm.chat_id
WHERE c.owner_id = @user_id::uuid
  AND (sqlc.narg('organization_id')::uuid IS NULL
       OR c.organization_id = sqlc.narg('organization_id')::uuid)
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
-- Returns -1 if no group limits match the specified scope.
-- When organization_id is NULL, groups across all organizations are
-- considered (global behavior). Otherwise only groups within the
-- specified organization are considered.
SELECT COALESCE(MIN(g.chat_spend_limit_micros), -1)::bigint AS limit_micros
FROM groups g
JOIN group_members_expanded gme ON gme.group_id = g.id
WHERE gme.user_id = @user_id::uuid
  AND (sqlc.narg('organization_id')::uuid IS NULL
       OR g.organization_id = sqlc.narg('organization_id')::uuid)
  AND g.chat_spend_limit_micros IS NOT NULL;

-- name: GetChatsByWorkspaceIDs :many
SELECT *
FROM chats_expanded
WHERE archived = false
  AND workspace_id = ANY(@ids::uuid[])
ORDER BY workspace_id, updated_at DESC;

-- name: ResolveUserChatSpendLimit :one
-- Resolves the effective spend limit for a user using the hierarchy:
-- 1. Individual user override (highest priority, applies globally across
--    all organizations since it lives on the users table)
-- 2. Minimum group limit across the user's groups
-- 3. Global default from config
-- Returns -1 if limits are not enabled.
-- When organization_id is NULL, groups across all organizations are
-- considered (global behavior). Otherwise only groups within the
-- specified organization are considered.
-- limit_source indicates which tier won: 'user', 'group', 'default',
-- or 'disabled'.
SELECT CASE
    WHEN NOT cfg.enabled THEN -1
    WHEN u.chat_spend_limit_micros IS NOT NULL THEN u.chat_spend_limit_micros
    WHEN gl.limit_micros IS NOT NULL THEN gl.limit_micros
    ELSE cfg.default_limit_micros
END::bigint AS effective_limit_micros,
CASE
    WHEN NOT cfg.enabled THEN 'disabled'
    WHEN u.chat_spend_limit_micros IS NOT NULL THEN 'user'
    WHEN gl.limit_micros IS NOT NULL THEN 'group'
    ELSE 'default'
END AS limit_source
FROM chat_usage_limit_config cfg
CROSS JOIN users u
LEFT JOIN LATERAL (
    SELECT MIN(g.chat_spend_limit_micros) AS limit_micros
    FROM groups g
    JOIN group_members_expanded gme ON gme.group_id = g.id
    WHERE gme.user_id = @user_id::uuid
      AND (sqlc.narg('organization_id')::uuid IS NULL
           OR g.organization_id = sqlc.narg('organization_id')::uuid)
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
-- All chat-scoped child tables are removed via ON DELETE CASCADE.
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
    c.id, c.owner_id, c.created_at, c.updated_at, c.status,
    (c.parent_chat_id IS NOT NULL)::bool AS has_parent,
    c.root_chat_id, c.workspace_id,
    c.mode, c.archived, c.last_model_config_id, c.client_type,
    cds.pull_request_state
FROM chats c
LEFT JOIN chat_diff_statuses cds ON cds.chat_id = c.id
WHERE c.updated_at > @updated_after;

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
-- deleted = false guarantees ai_provider_id is non-null, so INNER JOIN is safe.
SELECT cmc.id, ap.type::text AS provider, cmc.model, cmc.context_limit, cmc.enabled, cmc.is_default
FROM chat_model_configs cmc
JOIN ai_providers ap ON ap.id = cmc.ai_provider_id
WHERE cmc.deleted = false;
-- name: GetActiveChatsByAgentID :many
SELECT *
FROM chats_expanded
WHERE agent_id = @agent_id::uuid
    AND archived = false
    -- Active statuses only: waiting, pending, running, paused,
    -- requires_action.
    -- Excludes completed and error (terminal states).
    AND status IN ('waiting', 'running', 'paused', 'pending', 'requires_action')
ORDER BY updated_at DESC;

-- name: SoftDeleteContextFileMessages :exec
UPDATE chat_messages SET deleted = true
WHERE chat_id = @chat_id::uuid
    AND deleted = false
    AND content::jsonb @> '[{"type": "context-file"}]';

-- name: GetChatWorkerAcquisitionCandidates :many
-- Returns chats that workers may try to acquire. Candidates must be:
--   - in a worker-runnable execution status;
--   - unarchived; and
--   - missing ownership, carrying inconsistent ownership, or lacking a
--     fresh heartbeat for the assigned runner.
--
-- Missing ownership is worker_id IS NULL. Inconsistent ownership is
-- runner_id IS NULL while worker_id is set. Stale ownership is no
-- heartbeat row for (chat_id, runner_id), or one older than
-- @stale_seconds by database time. Candidates are ordered by oldest
-- updated_at first so workers drain stale runnable chats predictably.
SELECT
    chats_expanded.*,
    chat_heartbeats.heartbeat_at AS current_heartbeat_at,
    NOT EXISTS (
        SELECT 1
        FROM chat_heartbeats current_lease
        WHERE current_lease.chat_id = chats_expanded.id
          AND current_lease.runner_id = chats_expanded.runner_id
          AND current_lease.heartbeat_at > NOW() - (INTERVAL '1 second' * @stale_seconds::int)
    ) AS heartbeat_stale
FROM chats_expanded
LEFT JOIN chat_heartbeats
    ON chat_heartbeats.chat_id = chats_expanded.id
    AND chat_heartbeats.runner_id = chats_expanded.runner_id
WHERE
    chats_expanded.status IN ('running'::chat_status, 'interrupting'::chat_status, 'requires_action'::chat_status)
    AND chats_expanded.archived = false
    AND (
        chats_expanded.worker_id IS NULL
        OR chats_expanded.runner_id IS NULL
        OR NOT EXISTS (
            SELECT 1
            FROM chat_heartbeats current_lease
            WHERE current_lease.chat_id = chats_expanded.id
              AND current_lease.runner_id = chats_expanded.runner_id
              AND current_lease.heartbeat_at > NOW() - (INTERVAL '1 second' * @stale_seconds::int)
        )
    )
ORDER BY chats_expanded.updated_at ASC, chats_expanded.id ASC
LIMIT @limit_count::int;

-- name: GetChatsByIDsForRunnerSync :many
SELECT *
FROM chats_expanded
WHERE id = ANY(@ids::uuid[])
ORDER BY id ASC;

-- name: BatchUpsertChatHeartbeats :exec
INSERT INTO chat_heartbeats (chat_id, runner_id, heartbeat_at)
SELECT chat_ids.chat_id, runner_ids.runner_id, NOW()
FROM unnest(@chat_ids::uuid[]) WITH ORDINALITY AS chat_ids(chat_id, ord)
JOIN unnest(@runner_ids::uuid[]) WITH ORDINALITY AS runner_ids(runner_id, ord) USING (ord)
ON CONFLICT (chat_id, runner_id) DO UPDATE
SET heartbeat_at = EXCLUDED.heartbeat_at;

-- name: DeleteStaleChatHeartbeats :execrows
DELETE FROM chat_heartbeats
WHERE heartbeat_at < NOW() - (INTERVAL '1 second' * @stale_seconds::int);

-- name: GetAutoArchiveInactiveChatCandidates :many
-- Returns read-only root chat candidates for state-machine-backed
-- auto-archive. Activity is computed across the root family. The query
-- limits roots, not total family members.
SELECT
    chats_expanded.*,
    COALESCE(activity.last_activity_at, chats_expanded.created_at)::timestamptz AS last_activity_at
FROM chats_expanded
LEFT JOIN LATERAL (
    SELECT MAX(chat_messages.created_at) AS last_activity_at
    FROM chat_messages
    JOIN chats family_chat ON family_chat.id = chat_messages.chat_id
    WHERE (family_chat.id = chats_expanded.id OR family_chat.root_chat_id = chats_expanded.id)
      AND chat_messages.deleted = false
) activity ON TRUE
WHERE
    chats_expanded.archived = false
    AND chats_expanded.pin_order = 0
    AND chats_expanded.parent_chat_id IS NULL
    AND chats_expanded.created_at < @archive_cutoff::timestamptz
    AND chats_expanded.status NOT IN (
        'running'::chat_status,
        'interrupting'::chat_status,
        'pending'::chat_status,
        'paused'::chat_status,
        'requires_action'::chat_status
    )
    AND COALESCE(activity.last_activity_at, chats_expanded.created_at) < @archive_cutoff::timestamptz
ORDER BY chats_expanded.created_at ASC
LIMIT @limit_count::int;


-- name: LockChatAndBumpSnapshotVersion :one
-- Locks the chat row with FOR UPDATE and atomically increments its
-- snapshot_version, returning the post-bump chat. This is the single
-- entry point ChatMachine.Update uses to acquire the row lock and
-- allocate a new snapshot version in one round trip.
WITH bumped_chat AS (
    UPDATE chats
    SET snapshot_version = snapshot_version + 1
    WHERE id = (
        SELECT id FROM chats
        WHERE id = @id::uuid
        FOR UPDATE
    )
    RETURNING *
),
chats_expanded AS (
    SELECT
        bumped_chat.id,
        bumped_chat.owner_id,
        bumped_chat.workspace_id,
        bumped_chat.title,
        bumped_chat.status,
        bumped_chat.worker_id,
        bumped_chat.started_at,
        bumped_chat.heartbeat_at,
        bumped_chat.created_at,
        bumped_chat.updated_at,
        bumped_chat.parent_chat_id,
        bumped_chat.root_chat_id,
        bumped_chat.last_model_config_id,
        bumped_chat.archived,
        bumped_chat.last_error,
        bumped_chat.mode,
        bumped_chat.mcp_server_ids,
        bumped_chat.labels,
        bumped_chat.build_id,
        bumped_chat.agent_id,
        bumped_chat.pin_order,
        bumped_chat.last_read_message_id,
        bumped_chat.dynamic_tools,
        bumped_chat.organization_id,
        bumped_chat.plan_mode,
        bumped_chat.client_type,
        bumped_chat.last_turn_summary,
        bumped_chat.snapshot_version,
        bumped_chat.history_version,
        bumped_chat.queue_version,
        bumped_chat.generation_attempt,
        bumped_chat.retry_state,
        bumped_chat.retry_state_version,
        bumped_chat.runner_id,
        bumped_chat.requires_action_deadline_at,
        COALESCE(root.user_acl, bumped_chat.user_acl) AS user_acl,
        COALESCE(root.group_acl, bumped_chat.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        bumped_chat.context_aggregate_hash,
        bumped_chat.context_dirty_since,
        bumped_chat.context_dirty_resources,
        bumped_chat.context_error
    FROM bumped_chat
    LEFT JOIN chats root ON root.id = COALESCE(bumped_chat.root_chat_id, bumped_chat.parent_chat_id)
    JOIN visible_users owner ON owner.id = bumped_chat.owner_id
)
SELECT *
FROM chats_expanded;

-- name: UpdateChatExecutionState :one
-- Atomically updates the execution-state-managed fields on a chat:
-- status, archived, last_error, ownership identifiers, and the
-- requires-action deadline. Callers compose this with transition
-- mutations inside a single ChatMachine.Update transaction.
WITH updated_chat AS (
    UPDATE chats
    SET
        status = @status::chat_status,
        archived = @archived::boolean,
        worker_id = sqlc.narg('worker_id')::uuid,
        runner_id = sqlc.narg('runner_id')::uuid,
        last_error = sqlc.narg('last_error')::jsonb,
        requires_action_deadline_at = sqlc.narg('requires_action_deadline_at')::timestamptz,
        pin_order = CASE WHEN @archived::boolean THEN 0 ELSE pin_order END,
        updated_at = NOW()
    WHERE id = @id::uuid
    RETURNING *
),
chats_expanded AS (
    SELECT
        updated_chat.id,
        updated_chat.owner_id,
        updated_chat.workspace_id,
        updated_chat.title,
        updated_chat.status,
        updated_chat.worker_id,
        updated_chat.started_at,
        updated_chat.heartbeat_at,
        updated_chat.created_at,
        updated_chat.updated_at,
        updated_chat.parent_chat_id,
        updated_chat.root_chat_id,
        updated_chat.last_model_config_id,
        updated_chat.archived,
        updated_chat.last_error,
        updated_chat.mode,
        updated_chat.mcp_server_ids,
        updated_chat.labels,
        updated_chat.build_id,
        updated_chat.agent_id,
        updated_chat.pin_order,
        updated_chat.last_read_message_id,
        updated_chat.dynamic_tools,
        updated_chat.organization_id,
        updated_chat.plan_mode,
        updated_chat.client_type,
        updated_chat.last_turn_summary,
        updated_chat.snapshot_version,
        updated_chat.history_version,
        updated_chat.queue_version,
        updated_chat.generation_attempt,
        updated_chat.retry_state,
        updated_chat.retry_state_version,
        updated_chat.runner_id,
        updated_chat.requires_action_deadline_at,
        COALESCE(root.user_acl, updated_chat.user_acl) AS user_acl,
        COALESCE(root.group_acl, updated_chat.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        updated_chat.context_aggregate_hash,
        updated_chat.context_dirty_since,
        updated_chat.context_dirty_resources,
        updated_chat.context_error
    FROM updated_chat
    LEFT JOIN chats root ON root.id = COALESCE(updated_chat.root_chat_id, updated_chat.parent_chat_id)
    JOIN visible_users owner ON owner.id = updated_chat.owner_id
)
SELECT *
FROM chats_expanded;

-- name: UpdateChatRetryState :one
-- Stores the client-visible retry payload. retry_state_version is
-- assigned by trigger from the current snapshot_version.
WITH updated_chat AS (
    UPDATE chats
    SET
        retry_state = @retry_state::jsonb,
        updated_at = NOW()
    WHERE id = @id::uuid
    RETURNING *
),
chats_expanded AS (
    SELECT
        updated_chat.id,
        updated_chat.owner_id,
        updated_chat.workspace_id,
        updated_chat.title,
        updated_chat.status,
        updated_chat.worker_id,
        updated_chat.started_at,
        updated_chat.heartbeat_at,
        updated_chat.created_at,
        updated_chat.updated_at,
        updated_chat.parent_chat_id,
        updated_chat.root_chat_id,
        updated_chat.last_model_config_id,
        updated_chat.archived,
        updated_chat.last_error,
        updated_chat.mode,
        updated_chat.mcp_server_ids,
        updated_chat.labels,
        updated_chat.build_id,
        updated_chat.agent_id,
        updated_chat.pin_order,
        updated_chat.last_read_message_id,
        updated_chat.dynamic_tools,
        updated_chat.organization_id,
        updated_chat.plan_mode,
        updated_chat.client_type,
        updated_chat.last_turn_summary,
        updated_chat.snapshot_version,
        updated_chat.history_version,
        updated_chat.queue_version,
        updated_chat.generation_attempt,
        updated_chat.retry_state,
        updated_chat.retry_state_version,
        updated_chat.runner_id,
        updated_chat.requires_action_deadline_at,
        COALESCE(root.user_acl, updated_chat.user_acl) AS user_acl,
        COALESCE(root.group_acl, updated_chat.group_acl) AS group_acl,
        owner.username AS owner_username,
        owner.name AS owner_name,
        updated_chat.context_aggregate_hash,
        updated_chat.context_dirty_since,
        updated_chat.context_dirty_resources,
        updated_chat.context_error
    FROM updated_chat
    LEFT JOIN chats root ON root.id = COALESCE(updated_chat.root_chat_id, updated_chat.parent_chat_id)
    JOIN visible_users owner ON owner.id = updated_chat.owner_id
)
SELECT *
FROM chats_expanded;

-- name: IncrementChatGenerationAttempt :one
-- Increments generation_attempt and returns the resulting value.
UPDATE chats
SET generation_attempt = generation_attempt + 1, updated_at = NOW()
WHERE id = @id::uuid
RETURNING generation_attempt;

-- name: GetDatabaseNow :one
-- Returns the current database timestamp. Used so transitions that
-- record deadlines or heartbeats rely on a clock that is consistent
-- with the database rather than the caller's local clock.
SELECT NOW()::timestamptz AS now;

-- name: InsertChatQueuedMessageWithCreator :one
-- Inserts a queued message that carries a position (from the default
-- sequence) and an explicit created_by reference. Use this when the
-- queued-message creator differs from the chat owner.
INSERT INTO chat_queued_messages (chat_id, content, model_config_id, api_key_id, created_by)
VALUES (
    @chat_id::uuid,
    @content::jsonb,
    sqlc.narg('model_config_id')::uuid,
    sqlc.narg('api_key_id')::text,
    @created_by::uuid
)
RETURNING *;

-- name: GetChatQueuedMessagesByPosition :many
-- Returns queued messages in state-machine order (position ASC, id ASC).
SELECT * FROM chat_queued_messages
WHERE chat_id = @chat_id::uuid
ORDER BY position ASC, id ASC;

-- name: CountChatQueuedMessages :one
-- Cheap queue-length check used by ChatMachine.Update when deciding
-- whether the chat is in a "1" sub-state.
SELECT COUNT(*)::bigint AS count
FROM chat_queued_messages
WHERE chat_id = @chat_id::uuid;

-- name: GetChatQueuedMessageHead :one
-- Returns the queue head (lowest position, then lowest id).
SELECT * FROM chat_queued_messages
WHERE chat_id = @chat_id::uuid
ORDER BY position ASC, id ASC
LIMIT 1;

-- name: GetChatQueuedMessageByID :one
SELECT * FROM chat_queued_messages
WHERE id = @id::bigint AND chat_id = @chat_id::uuid;

-- name: DeleteChatQueuedMessageReturningCount :execrows
-- Deletes a queued message, scoped to the parent chat. Returns the
-- number of affected rows so callers can detect missing rows without
-- a follow-up read.
DELETE FROM chat_queued_messages
WHERE id = @id::bigint AND chat_id = @chat_id::uuid;

-- name: DeleteAllChatQueuedMessagesReturningCount :execrows
DELETE FROM chat_queued_messages
WHERE chat_id = @chat_id::uuid;

-- name: ReorderChatQueuedMessageToHead :execrows
-- Sets the target queued message's position to one less than the
-- current minimum position for that chat, moving it to the head.
UPDATE chat_queued_messages AS target
SET position = COALESCE(
    (SELECT MIN(position) FROM chat_queued_messages WHERE chat_id = @chat_id::uuid),
    0
) - 1
WHERE target.id = @id::bigint
  AND target.chat_id = @chat_id::uuid
  AND target.position > COALESCE(
    (SELECT MIN(position) FROM chat_queued_messages WHERE chat_id = @chat_id::uuid),
    target.position
  );

-- name: UpsertChatHeartbeat :exec
-- Upserts a heartbeat row for the (chat_id, runner_id) lease. Uses
-- database time so callers do not depend on a local clock.
INSERT INTO chat_heartbeats (chat_id, runner_id, heartbeat_at)
VALUES (@chat_id::uuid, @runner_id::uuid, NOW())
ON CONFLICT (chat_id, runner_id) DO UPDATE
SET heartbeat_at = EXCLUDED.heartbeat_at;

-- name: GetChatHeartbeat :one
SELECT * FROM chat_heartbeats
WHERE chat_id = @chat_id::uuid AND runner_id = @runner_id::uuid;

-- name: IsChatHeartbeatStale :one
-- Returns true when there is no heartbeat row for (chat_id, runner_id)
-- or the existing row is older than @stale_seconds seconds by database
-- time. chatstate calls this in a single query so the staleness check
-- is atomic and does not depend on the caller's local clock.
SELECT NOT EXISTS (
    SELECT 1 FROM chat_heartbeats
    WHERE chat_id = @chat_id::uuid
      AND runner_id = @runner_id::uuid
      AND heartbeat_at > NOW() - (INTERVAL '1 second' * @stale_seconds::int)
) AS stale;

-- name: BatchDeleteChatHeartbeats :execrows
-- Deletes heartbeat rows for the supplied (chat_id, runner_id) pairs.
DELETE FROM chat_heartbeats
USING unnest(@chat_ids::uuid[]) WITH ORDINALITY AS chat_ids(chat_id, ord)
JOIN unnest(@runner_ids::uuid[]) WITH ORDINALITY AS runner_ids(runner_id, ord) USING (ord)
WHERE chat_heartbeats.chat_id = chat_ids.chat_id
  AND chat_heartbeats.runner_id = runner_ids.runner_id;

-- name: DeleteAllChatHeartbeats :exec
-- Deletes all heartbeat rows for the chat. Used during ownership
-- transitions that abandon a lease.
DELETE FROM chat_heartbeats WHERE chat_id = @chat_id::uuid;


-- name: GetChatStreamSyncRows :many
SELECT
    id,
    snapshot_version,
    history_version,
    queue_version,
    retry_state_version,
    generation_attempt,
    status,
    worker_id
FROM chats
WHERE id = ANY(@ids::uuid[])
ORDER BY id ASC;

-- name: AutoArchiveInactiveChats :many
-- Archives inactive root chats (pinned and already-archived chats skipped),
-- cascading to children via root_chat_id. Limits apply to roots, not total
-- rows. The Go caller passes @archive_cutoff as UTC midnight so that all
-- chats sharing the same last-activity date are archived together.
-- Used by dbpurge.
WITH to_archive AS (
    SELECT
        c.id,
        -- Activity = MAX(cm.created_at) across the family, or c.created_at
        -- when the family has no non-deleted messages.
        COALESCE(activity.last_activity_at, c.created_at) AS last_activity_at
    FROM chats c
    LEFT JOIN LATERAL (
        SELECT MAX(cm.created_at) AS last_activity_at
        FROM chat_messages cm
        JOIN chats fc ON fc.id = cm.chat_id
        WHERE (fc.id = c.id OR fc.root_chat_id = c.id)
          AND cm.deleted = false
    ) activity ON TRUE
    WHERE c.archived = false
      AND c.pin_order = 0
      AND c.parent_chat_id IS NULL -- roots only
      -- Redundant filter helps the planner use the partial index on created_at.
      AND c.created_at < @archive_cutoff::timestamptz
      -- New active statuses must be added here to prevent archiving.
      AND c.status NOT IN ('running', 'pending', 'paused', 'requires_action')
      AND COALESCE(activity.last_activity_at, c.created_at) < @archive_cutoff::timestamptz
    -- Sorting by created_at lets Postgres drive the scan from the
    -- partial index instead of evaluating every LATERAL subquery
    -- before sorting. All candidates are past the cutoff, so the
    -- archive order is immaterial once the backlog drains.
    ORDER BY c.created_at ASC
    LIMIT @limit_count
),
archived AS (
    UPDATE chats c
    SET archived = true, pin_order = 0, updated_at = NOW()
    FROM to_archive t
    WHERE (c.id = t.id OR c.root_chat_id = t.id) -- cascade to children
      AND c.archived = false
    RETURNING c.*
)
SELECT
    a.*,
    -- Children inherit their root's activity so last_activity_at is never null.
    COALESCE(
        t.last_activity_at,
        (SELECT tr.last_activity_at FROM to_archive tr WHERE tr.id = a.root_chat_id),
        a.created_at
    )::timestamptz AS last_activity_at
FROM archived a
LEFT JOIN to_archive t ON t.id = a.id
-- created_at ASC flows through to dbpurge's digest truncation; see
-- buildDigestData in dbpurge.go for the tradeoff rationale.
ORDER BY (a.root_chat_id IS NULL) DESC, a.owner_id ASC, a.created_at ASC, a.id ASC;
