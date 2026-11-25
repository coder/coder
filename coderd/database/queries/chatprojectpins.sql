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
