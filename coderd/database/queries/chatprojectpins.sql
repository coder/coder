-- name: InsertChatProjectPin :exec
-- Pins a newly created project for its creator at the end of their list.
INSERT INTO chat_project_pins (user_id, project_id, pin_order, source)
SELECT @user_id::uuid, @project_id::uuid, COALESCE(MAX(pin_order), 0) + 1, 'default'
FROM chat_project_pins
WHERE user_id = @user_id::uuid AND unpinned_at IS NULL;

-- name: PinChatProject :exec
-- Pins a project, or moves an already pinned project, to pin_order. Without
-- pin_order a new pin goes to the end and an existing pin keeps its place.
-- Positions count only pins the user can still see, as the summaries do, and
-- are clamped to [1, count]. Pins on projects the user can no longer see, or
-- on archived projects, move after the visible ones. Callers must hold the per-user pin lock so
-- concurrent writes cannot interleave.
WITH current_pins AS (
    SELECT project_id, stale, ROW_NUMBER() OVER (ORDER BY stale ASC, pin_order ASC, project_id ASC) :: integer AS position
    FROM (
        SELECT p.project_id, p.pin_order, cp.archived_at IS NOT NULL OR NOT (
            -- Same access predicate as GetChatProjectSummariesByUserID.
            cp.owner_id = @user_id::uuid
            OR (
                @acl_enabled::boolean
                AND (cp.user_acl ? (@user_id::uuid)::text OR cp.group_acl ?| ARRAY(
                    SELECT group_id::text FROM group_members WHERE user_id = @user_id::uuid
                    UNION ALL
                    SELECT organization_id::text FROM organization_members WHERE user_id = @user_id::uuid
                ))
                AND EXISTS (
                    SELECT 1 FROM organization_members
                    WHERE organization_members.user_id = @user_id::uuid
                        AND organization_members.organization_id = cp.organization_id
                )
            )
        ) AS stale
        FROM chat_project_pins p
        JOIN chat_projects cp ON cp.id = p.project_id
        WHERE p.user_id = @user_id::uuid AND p.unpinned_at IS NULL
    ) pins
),
ranked AS (
    SELECT project_id, stale, ROW_NUMBER() OVER (ORDER BY position ASC) :: integer AS position
    FROM current_pins
    WHERE project_id <> @project_id::uuid
),
target AS (
    SELECT LEAST(GREATEST(COALESCE(
        sqlc.narg('pin_order')::integer,
        (SELECT position FROM current_pins WHERE project_id = @project_id::uuid),
        pinned_count + 1
    ), 1), pinned_count + 1) :: integer AS position
    FROM (SELECT COUNT(*) FILTER (WHERE NOT stale) :: integer AS pinned_count FROM ranked) counts
),
shifted AS (
    UPDATE chat_project_pins p
    SET pin_order = CASE WHEN ranked.position >= target.position THEN ranked.position + 1 ELSE ranked.position END
    FROM ranked CROSS JOIN target
    WHERE p.user_id = @user_id::uuid AND p.project_id = ranked.project_id AND p.unpinned_at IS NULL
)
INSERT INTO chat_project_pins (user_id, project_id, pin_order, source)
SELECT @user_id::uuid, @project_id::uuid, target.position, 'user'
FROM target
ON CONFLICT (user_id, project_id) DO UPDATE SET
    pin_order = EXCLUDED.pin_order,
    source = 'user',
    unpinned_at = NULL;

-- name: UnpinChatProject :exec
-- Turns a pin into a tombstone and compacts the remaining pins to 1..N.
-- The source is kept so telemetry can tell which pins users removed.
-- Unpinning a project that is not pinned changes nothing. Like
-- PinChatProject, pins the user can still see on unarchived projects come
-- first, and callers must hold the per-user pin lock.
WITH target AS (
    SELECT project_id
    FROM chat_project_pins
    WHERE user_id = @user_id::uuid AND project_id = @project_id::uuid AND unpinned_at IS NULL
),
updates AS (
    SELECT p.project_id, ROW_NUMBER() OVER (ORDER BY cp.archived_at IS NOT NULL OR NOT (
        -- Same access predicate as GetChatProjectSummariesByUserID.
        cp.owner_id = @user_id::uuid
        OR (
            @acl_enabled::boolean
            AND (cp.user_acl ? (@user_id::uuid)::text OR cp.group_acl ?| ARRAY(
                SELECT group_id::text FROM group_members WHERE user_id = @user_id::uuid
                UNION ALL
                SELECT organization_id::text FROM organization_members WHERE user_id = @user_id::uuid
            ))
            AND EXISTS (
                SELECT 1 FROM organization_members
                WHERE organization_members.user_id = @user_id::uuid
                    AND organization_members.organization_id = cp.organization_id
            )
        )
    ) ASC, p.pin_order ASC, p.project_id ASC) :: integer AS pin_order, NULL::timestamptz AS unpinned_at
    FROM chat_project_pins p
    JOIN chat_projects cp ON cp.id = p.project_id
    WHERE p.user_id = @user_id::uuid AND p.unpinned_at IS NULL AND p.project_id <> @project_id::uuid
    UNION ALL
    SELECT project_id, 0, NOW() FROM target
)
UPDATE chat_project_pins p
SET pin_order = updates.pin_order, unpinned_at = updates.unpinned_at
FROM updates
WHERE p.user_id = @user_id::uuid AND p.project_id = updates.project_id AND p.unpinned_at IS NULL
    AND EXISTS (SELECT 1 FROM target);

-- name: GetChatProjectSummariesByUserID :many
-- Returns the unarchived projects from GetChatProjectsOwnedOrSharedWithUserID
-- with the user's active pin and aggregates over the user's own top-level,
-- non-archived chats. Entries match regardless of the actions they grant,
-- so callers must authorize each row; pins on projects the user can no
-- longer see, or on archived projects, are never returned.
WITH accessible AS (
    SELECT id, organization_id, owner_id, user_acl, group_acl
    FROM chat_projects
    WHERE archived_at IS NULL AND (owner_id = @user_id::uuid
        OR (
            -- Grants count only while RBAC honors them, as in
            -- GetChatProjectsOwnedOrSharedWithUserID after authorization.
            @acl_enabled::boolean
            AND (user_acl ? (@user_id::uuid)::text OR group_acl ?| ARRAY(
                SELECT group_id::text FROM group_members WHERE user_id = @user_id::uuid
                UNION ALL
                SELECT organization_id::text FROM organization_members WHERE user_id = @user_id::uuid
            ))
            AND EXISTS (
                SELECT 1 FROM organization_members
                WHERE organization_members.user_id = @user_id::uuid
                    AND organization_members.organization_id = chat_projects.organization_id
            )
        ))
),
-- Users who can use each project: the owner, granted users who are
-- organization members, and members of granted groups (including the
-- Everyone group). Counted only while project sharing is available;
-- otherwise every project counts its owner alone. Suspended users cannot
-- sign in, so they are not counted.
project_members AS (
    SELECT id AS project_id, owner_id AS user_id FROM accessible
    UNION
    SELECT granted.project_id, organization_members.user_id
    FROM (
        SELECT id AS project_id, organization_id, jsonb_object_keys(user_acl)::uuid AS user_id
        FROM accessible WHERE @count_shared_members::boolean
    ) granted
    JOIN organization_members
        ON organization_members.user_id = granted.user_id
        AND organization_members.organization_id = granted.organization_id
    UNION
    SELECT granted.project_id, group_members_expanded.user_id
    FROM (
        SELECT id AS project_id, organization_id, jsonb_object_keys(group_acl)::uuid AS group_id
        FROM accessible WHERE @count_shared_members::boolean
    ) granted
    JOIN group_members_expanded ON group_members_expanded.group_id = granted.group_id
    -- A granted group from another organization only reaches its members
    -- who also belong to the project's organization.
    JOIN organization_members
        ON organization_members.user_id = group_members_expanded.user_id
        AND organization_members.organization_id = granted.organization_id
),
member_counts AS (
    SELECT project_members.project_id, COUNT(*) AS member_count
    FROM project_members
    JOIN users ON users.id = project_members.user_id
    WHERE users.deleted = false AND users.is_system = false AND users.status <> 'suspended'
    GROUP BY project_members.project_id
),
-- Read through idx_chats_owner.
own_chats AS (
    SELECT
        c.project_id,
        COUNT(*) AS chat_count,
        -- Same rule as has_unread in GetChats.
        COUNT(*) FILTER (WHERE EXISTS (
            SELECT 1 FROM chat_messages cm
            WHERE cm.chat_id = c.id AND cm.role = 'assistant' AND cm.deleted = false
                AND cm.id > COALESCE(c.last_read_message_id, 0)
        )) AS unread_count,
        bool_or(c.status IN ('running', 'interrupting')) AS running,
        bool_or(c.status IN ('requires_action', 'error', 'paused')) AS needs_attention,
        MAX(c.updated_at)::timestamptz AS last_activity_at
    FROM chats c
    WHERE c.owner_id = @user_id::uuid AND c.project_id IS NOT NULL AND c.parent_chat_id IS NULL AND c.archived = false
    GROUP BY c.project_id
)
SELECT
    sqlc.embed(chat_projects),
    (pins.project_id IS NOT NULL)::boolean AS pinned,
    -- Numbered over the returned pins, so pins the user can no longer see
    -- leave no gaps.
    (CASE WHEN pins.project_id IS NULL THEN 0
        ELSE ROW_NUMBER() OVER (PARTITION BY pins.project_id IS NULL ORDER BY pins.pin_order, chat_projects.id) END)::integer AS pin_order,
    COALESCE(own_chats.chat_count, 0)::bigint AS own_chat_count,
    COALESCE(own_chats.unread_count, 0)::bigint AS own_unread_count,
    COALESCE(own_chats.running, false)::boolean AS own_running,
    COALESCE(own_chats.needs_attention, false)::boolean AS own_needs_attention,
    own_chats.last_activity_at,
    (CASE WHEN @count_shared_members::boolean THEN COALESCE(member_counts.member_count, 0) ELSE 1 END)::bigint AS member_count
FROM chat_projects
JOIN accessible ON accessible.id = chat_projects.id
LEFT JOIN member_counts ON member_counts.project_id = chat_projects.id
LEFT JOIN chat_project_pins pins
    ON pins.user_id = @user_id::uuid AND pins.project_id = chat_projects.id AND pins.unpinned_at IS NULL
LEFT JOIN own_chats ON own_chats.project_id = chat_projects.id
ORDER BY lower(chat_projects.name), chat_projects.id;

-- name: GetChatProjectPinTelemetry :one
-- Aggregates pin usage over each active, non-system user's accessible
-- projects. Grants count only when @acl_enabled, matching the summaries.
-- No identifiers leave this query.
WITH project_users AS (
    SELECT id AS project_id, owner_id AS user_id FROM chat_projects
    UNION
    SELECT granted.project_id, organization_members.user_id
    FROM (
        SELECT id AS project_id, organization_id, jsonb_object_keys(user_acl)::uuid AS user_id FROM chat_projects
    ) granted
    JOIN organization_members
        ON organization_members.user_id = granted.user_id
        AND organization_members.organization_id = granted.organization_id
    WHERE @acl_enabled::boolean
    UNION
    SELECT granted.project_id, group_members_expanded.user_id
    FROM (
        SELECT id AS project_id, organization_id, jsonb_object_keys(group_acl)::uuid AS group_id FROM chat_projects
    ) granted
    JOIN group_members_expanded ON group_members_expanded.group_id = granted.group_id
    JOIN organization_members
        ON organization_members.user_id = group_members_expanded.user_id
        AND organization_members.organization_id = granted.organization_id
    WHERE @acl_enabled::boolean
),
per_user AS (
    SELECT
        COUNT(*) AS projects,
        COUNT(*) FILTER (WHERE pins.pin_order > 0) AS pinned,
        COUNT(*) FILTER (WHERE pins.pin_order > 0 AND pins.source = 'migration') AS pinned_migration,
        COUNT(*) FILTER (WHERE pins.pin_order > 0 AND pins.source = 'default') AS pinned_default,
        COUNT(*) FILTER (WHERE pins.pin_order > 0 AND pins.source = 'user') AS pinned_user,
        COUNT(*) FILTER (WHERE pins.unpinned_at IS NOT NULL AND pins.source = 'migration') AS tombstones_migration,
        COUNT(*) FILTER (WHERE pins.unpinned_at IS NOT NULL AND pins.source = 'default') AS tombstones_default,
        COUNT(*) FILTER (WHERE pins.unpinned_at IS NOT NULL AND pins.source = 'user') AS tombstones_user,
        COUNT(*) FILTER (WHERE pins.unpinned_at IS NOT NULL) AS tombstones
    FROM project_users
    JOIN users ON users.id = project_users.user_id
        AND users.status = 'active'::user_status AND users.deleted = false AND users.is_system = false
    LEFT JOIN chat_project_pins pins
        ON pins.user_id = project_users.user_id AND pins.project_id = project_users.project_id
    GROUP BY project_users.user_id
),
-- Users act on pins by pinning, reordering, or unpinning a project.
bucketed AS (
    SELECT
        *,
        tombstones > 0 OR pinned_user > 0 AS acted,
        CASE
            WHEN pinned = 0 THEN 0
            WHEN pinned = projects THEN 100
            WHEN pinned * 4 <= projects THEN 25
            WHEN pinned * 2 <= projects THEN 50
            WHEN pinned * 4 <= projects * 3 THEN 75
            ELSE 99
        END AS bucket
    FROM per_user
)
SELECT
    COUNT(*)::bigint AS users_with_projects,
    COUNT(*) FILTER (WHERE tombstones > 0)::bigint AS users_with_tombstones,
    COALESCE(SUM(pinned_migration), 0)::bigint AS pinned_migration,
    COALESCE(SUM(pinned_default), 0)::bigint AS pinned_default,
    COALESCE(SUM(pinned_user), 0)::bigint AS pinned_user,
    COALESCE(SUM(tombstones_migration), 0)::bigint AS tombstones_migration,
    COALESCE(SUM(tombstones_default), 0)::bigint AS tombstones_default,
    COALESCE(SUM(tombstones_user), 0)::bigint AS tombstones_user,
    COUNT(*) FILTER (WHERE bucket = 0)::bigint AS all_share_0,
    COUNT(*) FILTER (WHERE bucket = 25)::bigint AS all_share_1_25,
    COUNT(*) FILTER (WHERE bucket = 50)::bigint AS all_share_26_50,
    COUNT(*) FILTER (WHERE bucket = 75)::bigint AS all_share_51_75,
    COUNT(*) FILTER (WHERE bucket = 99)::bigint AS all_share_76_99,
    COUNT(*) FILTER (WHERE bucket = 100)::bigint AS all_share_100,
    COUNT(*) FILTER (WHERE acted AND bucket = 0)::bigint AS acted_share_0,
    COUNT(*) FILTER (WHERE acted AND bucket = 25)::bigint AS acted_share_1_25,
    COUNT(*) FILTER (WHERE acted AND bucket = 50)::bigint AS acted_share_26_50,
    COUNT(*) FILTER (WHERE acted AND bucket = 75)::bigint AS acted_share_51_75,
    COUNT(*) FILTER (WHERE acted AND bucket = 99)::bigint AS acted_share_76_99,
    COUNT(*) FILTER (WHERE acted AND bucket = 100)::bigint AS acted_share_100
FROM bucketed;
