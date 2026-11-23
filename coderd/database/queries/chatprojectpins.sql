-- name: InsertChatProjectPin :exec
-- Pins a newly created project for its creator at the end of their list.
INSERT INTO chat_project_pins (user_id, project_id, pin_order, source)
SELECT @user_id::uuid, @project_id::uuid, COALESCE(MAX(pin_order), 0) + 1, 'default'
FROM chat_project_pins
WHERE user_id = @user_id::uuid;

-- name: PinChatProject :exec
-- Pins a project, or moves an already pinned project, to pin_order. Without
-- pin_order a new pin goes to the end and an existing pin keeps its place.
-- Positions count the pins GetChatProjectSummariesByUserID matches and are
-- clamped to [1, count], so callers must translate a position the user sees
-- past matched pins that authorization hides. Pins the summaries no longer
-- match move after the others. Callers must hold the per-user pin lock so
-- concurrent writes cannot interleave.
WITH current_pins AS (
    SELECT project_id, stale, ROW_NUMBER() OVER (ORDER BY stale ASC, pin_order ASC, project_id ASC) :: integer AS position
    FROM (
        SELECT p.project_id, p.pin_order, p.project_id NOT IN (
            SELECT project_id FROM chat_project_ids_for_user(@user_id::uuid, true)
        ) AS stale
        FROM chat_project_pins p
        WHERE p.user_id = @user_id::uuid
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
    WHERE p.user_id = @user_id::uuid AND p.project_id = ranked.project_id
)
INSERT INTO chat_project_pins (user_id, project_id, pin_order, source)
SELECT @user_id::uuid, @project_id::uuid, target.position, 'user'
FROM target
ON CONFLICT (user_id, project_id) DO UPDATE SET
    pin_order = EXCLUDED.pin_order,
    source = 'user';

-- name: UnpinChatProject :exec
-- Deletes a pin and compacts the remaining pins to 1..N.
-- Unpinning a project that is not pinned changes nothing. Like
-- PinChatProject, pins the summaries still match come first, and callers
-- must hold the per-user pin lock.
WITH deleted AS (
    DELETE FROM chat_project_pins
    WHERE user_id = @user_id::uuid AND project_id = @project_id::uuid
    RETURNING project_id
),
updates AS (
    SELECT p.project_id, ROW_NUMBER() OVER (ORDER BY p.project_id NOT IN (
        SELECT project_id FROM chat_project_ids_for_user(@user_id::uuid, true)
    ) ASC, p.pin_order ASC, p.project_id ASC) :: integer AS pin_order
    FROM chat_project_pins p
    WHERE p.user_id = @user_id::uuid AND p.project_id <> @project_id::uuid
)
UPDATE chat_project_pins p
SET pin_order = updates.pin_order
FROM updates
WHERE p.user_id = @user_id::uuid AND p.project_id = updates.project_id
    AND EXISTS (SELECT 1 FROM deleted);

-- name: GetChatProjectSummariesByUserID :many
-- Returns the projects from GetChatProjectsOwnedOrSharedWithUserID with the
-- user's pin and aggregates over the user's own top-level,
-- non-archived chats. Entries match regardless of the actions they grant,
-- so callers must authorize each row; pins on projects the user can no
-- longer see are never returned.
WITH accessible AS (
    -- Grant keys select candidates even while chat sharing is disabled, as
    -- in GetChatProjectsOwnedOrSharedWithUserID: a role can still let the
    -- caller read the project.
    SELECT id, organization_id, owner_id, user_acl, group_acl
    FROM chat_projects
    WHERE id IN (SELECT project_id FROM chat_project_ids_for_user(@user_id::uuid, true))
),
-- Users who can use each project: the owner and the users its read grants
-- reach (chat_project_read_grant_members). Counted only while project
-- sharing is available;
-- otherwise every project counts its owner alone. Suspended users cannot
-- sign in, so they are not counted.
--
-- A read grant to the Everyone group of the project's organization reaches
-- every member, and the other grants reach only members, so such a project
-- counts its organization's members once per organization instead of
-- expanding the group into a row per member for every project.
everyone_projects AS (
    SELECT id AS project_id, organization_id FROM accessible
    WHERE @count_shared_members::boolean
        AND group_acl -> organization_id::text -> 'permissions' ?| ARRAY['read', '*']
),
org_member_counts AS (
    SELECT organization_members.organization_id, COUNT(*) AS member_count
    FROM organization_members
    JOIN users ON users.id = organization_members.user_id
    WHERE organization_members.organization_id IN (SELECT organization_id FROM everyone_projects)
        AND users.deleted = false AND users.is_system = false AND users.status <> 'suspended'
    GROUP BY organization_members.organization_id
),
project_members AS (
    -- org_member_counts already counts the owner of an Everyone project who
    -- is a member of its organization.
    SELECT id AS project_id, owner_id AS user_id FROM accessible
    WHERE id NOT IN (SELECT project_id FROM everyone_projects)
        OR NOT EXISTS (
            SELECT 1 FROM organization_members
            WHERE organization_members.user_id = accessible.owner_id
                AND organization_members.organization_id = accessible.organization_id
        )
    UNION
    SELECT project_id, user_id FROM chat_project_read_grant_members
    -- An array lets the planner filter the view's project scans; a join or
    -- IN would expand the grants of every project first.
    WHERE @count_shared_members::boolean AND project_id = ANY(ARRAY(
        SELECT id FROM accessible
        EXCEPT
        SELECT project_id FROM everyone_projects
    ))
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
    (CASE WHEN @count_shared_members::boolean THEN COALESCE(member_counts.member_count, 0) + COALESCE(org_member_counts.member_count, 0) ELSE 1 END)::bigint AS member_count
FROM chat_projects
JOIN accessible ON accessible.id = chat_projects.id
LEFT JOIN member_counts ON member_counts.project_id = chat_projects.id
LEFT JOIN everyone_projects ON everyone_projects.project_id = chat_projects.id
LEFT JOIN org_member_counts ON org_member_counts.organization_id = everyone_projects.organization_id
LEFT JOIN chat_project_pins pins
    ON pins.user_id = @user_id::uuid AND pins.project_id = chat_projects.id
LEFT JOIN own_chats ON own_chats.project_id = chat_projects.id
ORDER BY lower(chat_projects.name), chat_projects.id;
