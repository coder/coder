-- name: InsertChatProjectPin :exec
-- Pins a newly created project for its creator at the end of their list.
INSERT INTO chat_project_pins (user_id, project_id, pin_order, source)
SELECT @user_id::uuid, @project_id::uuid, COALESCE(MAX(pin_order), 0) + 1, 'default'
FROM chat_project_pins
WHERE user_id = @user_id::uuid AND unpinned_at IS NULL;

-- name: PinChatProject :exec
-- Pins a project, or moves an already pinned project, to pin_order. Without
-- pin_order a new pin goes to the end and an existing pin keeps its place.
-- Positions count only pins the user can still see and are clamped to
-- [1, count]. Pins on projects the user can no longer see move
-- after the visible ones. Callers must hold the per-user pin lock so
-- concurrent writes cannot interleave.
WITH current_pins AS (
    SELECT project_id, stale, ROW_NUMBER() OVER (ORDER BY stale ASC, pin_order ASC, project_id ASC) :: integer AS position
    FROM (
        SELECT p.project_id, p.pin_order, NOT (
            -- The user can see a project they own, or one granted to them or
            -- their groups while grants are honored and they are a member of
            -- its organization.
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
-- PinChatProject, pins the user can still see come first, and callers must
-- hold the per-user pin lock.
WITH target AS (
    SELECT project_id
    FROM chat_project_pins
    WHERE user_id = @user_id::uuid AND project_id = @project_id::uuid AND unpinned_at IS NULL
),
updates AS (
    SELECT p.project_id, ROW_NUMBER() OVER (ORDER BY NOT (
        -- Same access predicate as PinChatProject.
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
