-- name: FetchUnreadInboxNotificationsByUserID :many
SELECT * FROM notifications_inbox WHERE user_id = $1 AND read_at IS NULL ORDER BY created_at DESC;

-- name: FetchInboxNotificationsByUserID :many
SELECT * FROM notifications_inbox WHERE user_id = $1 ORDER BY created_at DESC;

-- name: FetchInboxNotificationsByUserIDAndTemplateIDAndTargetID :many
SELECT * FROM notifications_inbox WHERE user_id = $1 AND template_id = $2 AND target_id = $3 ORDER BY created_at DESC;

-- name: FetchUnreadInboxNotificationsByUserIDAndTemplateIDAndTargetID :many
SELECT * FROM notifications_inbox WHERE user_id = $1 AND template_id = $2 AND target_id = $3 AND read_at IS NULL ORDER BY created_at DESC;

-- name: GetInboxNotificationByID :one
SELECT * FROM notifications_inbox WHERE id = $1;

-- name: InsertInboxNotification :one
INSERT INTO
    notifications_inbox (
        id,
		user_id,
		template_id,
		target_id,
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
	notifications_inbox
SET
	read_at = $1
WHERE
	id = @id;
