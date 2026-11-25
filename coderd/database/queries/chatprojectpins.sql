-- name: PinChatProject :exec
-- Pins a project at the end of the user's list. An existing pin keeps its
-- place. Callers must hold the per-user pin lock, or two new pins could
-- take the same position.
INSERT INTO chat_project_pins (user_id, project_id, pin_order, source)
SELECT @user_id::uuid, @project_id::uuid, COALESCE(MAX(pin_order), 0) + 1, @source::chat_project_pin_source
FROM chat_project_pins
WHERE user_id = @user_id::uuid
ON CONFLICT (user_id, project_id) DO NOTHING;

-- name: ReorderChatProjectPins :exec
-- Puts the user's pins on project_ids in that order, followed by the user's
-- other pins in their current order. IDs the user has not pinned are
-- ignored. The other pins include those on projects the user can no longer
-- see, so no caller has to work out which pins are hidden. Callers must hold
-- the per-user pin lock, or overlapping reorders could deadlock.
WITH ranked AS (
    SELECT p.project_id, ROW_NUMBER() OVER (
        ORDER BY array_position(@project_ids::uuid[], p.project_id) ASC NULLS LAST, p.pin_order ASC, p.project_id ASC
    ) :: integer AS pin_order
    FROM chat_project_pins p
    WHERE p.user_id = @user_id::uuid
)
UPDATE chat_project_pins p
SET pin_order = ranked.pin_order
FROM ranked
WHERE p.user_id = @user_id::uuid AND p.project_id = ranked.project_id;

-- name: UnpinChatProject :exec
-- Deletes a pin. Unpinning a project that is not pinned changes nothing.
-- The remaining pins keep their order, with a gap in pin_order until the
-- next reorder.
DELETE FROM chat_project_pins
WHERE user_id = @user_id::uuid AND project_id = @project_id::uuid;

-- name: GetChatProjectSummariesByUserID :many
-- Returns the user's pin and aggregates over the user's own top-level,
-- non-archived chats for each project from
-- GetChatProjectsOwnedOrSharedWithUserID. The project columns only serve
-- authorization: entries match regardless of the actions they grant, so
-- callers must authorize each row. Pins on projects the user can no longer
-- see are never returned.
WITH own_chats AS (
    -- Read through idx_chats_owner.
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
    chat_projects.id AS project_id,
    chat_projects.organization_id,
    chat_projects.owner_id,
    chat_projects.user_acl,
    chat_projects.group_acl,
    (pins.project_id IS NOT NULL)::boolean AS pinned,
    -- Numbered over the returned pins, so pins the user can no longer see
    -- leave no gaps.
    (CASE WHEN pins.project_id IS NULL THEN 0
        ELSE ROW_NUMBER() OVER (PARTITION BY pins.project_id IS NULL ORDER BY pins.pin_order, chat_projects.id) END)::integer AS pin_order,
    COALESCE(own_chats.chat_count, 0)::bigint AS own_chat_count,
    COALESCE(own_chats.unread_count, 0)::bigint AS own_unread_count,
    COALESCE(own_chats.running, false)::boolean AS own_running,
    COALESCE(own_chats.needs_attention, false)::boolean AS own_needs_attention,
    own_chats.last_activity_at
FROM chat_projects
LEFT JOIN chat_project_pins pins
    ON pins.user_id = @user_id::uuid AND pins.project_id = chat_projects.id
LEFT JOIN own_chats ON own_chats.project_id = chat_projects.id
-- Grant keys select candidates even while chat sharing is disabled, as in
-- GetChatProjectsOwnedOrSharedWithUserID: a role can still let the caller
-- read the project.
WHERE chat_projects.id IN (SELECT project_id FROM chat_project_ids_for_user(@user_id::uuid, true))
ORDER BY lower(chat_projects.name), chat_projects.id;

-- name: GetChatProjectMemberCountsByUserID :many
-- Returns how many users can use each project from
-- GetChatProjectsOwnedOrSharedWithUserID: the owner and the users its read
-- grants reach (chat_project_read_grant_members). Grants are counted only
-- while project sharing is available; otherwise every project counts its
-- owner alone. Suspended users cannot sign in, so they are not counted. The
-- project columns only serve authorization: callers must authorize each
-- row.
WITH accessible AS (
    SELECT id, organization_id, owner_id, group_acl
    FROM chat_projects
    WHERE id IN (SELECT project_id FROM chat_project_ids_for_user(@user_id::uuid, true))
),
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
)
SELECT
    chat_projects.id AS project_id,
    chat_projects.organization_id,
    chat_projects.owner_id,
    chat_projects.user_acl,
    chat_projects.group_acl,
    (CASE WHEN @count_shared_members::boolean THEN COALESCE(member_counts.member_count, 0) + COALESCE(org_member_counts.member_count, 0) ELSE 1 END)::bigint AS member_count
FROM chat_projects
JOIN accessible ON accessible.id = chat_projects.id
LEFT JOIN member_counts ON member_counts.project_id = chat_projects.id
LEFT JOIN everyone_projects ON everyone_projects.project_id = chat_projects.id
LEFT JOIN org_member_counts ON org_member_counts.organization_id = everyone_projects.organization_id
ORDER BY chat_projects.id;
