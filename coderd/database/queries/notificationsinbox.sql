-- name: FetchUnreadInboxNotificationsByUserID :many
SELECT * FROM inbox_notifications WHERE user_id = $1 AND read_at IS NULL ORDER BY created_at DESC;

-- name: FetchInboxNotificationsByUserID :many
SELECT * FROM inbox_notifications WHERE user_id = $1 ORDER BY created_at DESC;

-- name: FetchInboxNotificationsByUserIDFilteredByTemplatesAndTargets :many
SELECT * FROM inbox_notifications WHERE user_id = @user_id AND template_id = ANY(@templates::UUID[]) AND targets @> COALESCE(@targets, ARRAY[]::UUID[]) ORDER BY created_at DESC;

-- name: FetchUnreadInboxNotificationsByUserIDFilteredByTemplatesAndTargets :many
SELECT * FROM inbox_notifications WHERE user_id = @user_id AND template_id = ANY(@templates::UUID[]) AND targets @> COALESCE(@targets, ARRAY[]::UUID[]) AND read_at IS NULL ORDER BY created_at DESC;

-- name: GetInboxNotificationByID :one
SELECT * FROM inbox_notifications WHERE id = $1;

-- name: InsertInboxNotification :one
INSERT INTO
    inbox_notifications (
        id,
        user_id,
        template_id,
        targets,
        title,
        content,
        icon,
        actions,
        created_at
    )
VALUES
    ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: SetInboxNotificationAsRead :exec
UPDATE
    inbox_notifications
SET
    read_at = $1
WHERE
    id = $2;
