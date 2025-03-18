-- name: GetInboxNotificationsByUserID :many
-- Fetches inbox notifications for a user filtered by templates and targets
-- param user_id: The user ID
-- param read_status: The read status to filter by - can be any of 'ALL', 'UNREAD', 'READ'
-- param created_at_opt: The created_at timestamp to filter by. This parameter is usd for pagination - it fetches notifications created before the specified timestamp if it is not the zero value
-- param limit_opt: The limit of notifications to fetch. If the limit is not specified, it defaults to 25
SELECT * FROM inbox_notifications WHERE
	user_id = @user_id AND
	(@read_status::inbox_notification_read_status = 'all' OR (@read_status::inbox_notification_read_status = 'unread' AND read_at IS NULL) OR (@read_status::inbox_notification_read_status = 'read' AND read_at IS NOT NULL)) AND
	(@created_at_opt::TIMESTAMPTZ = '0001-01-01 00:00:00Z' OR created_at < @created_at_opt::TIMESTAMPTZ)
	ORDER BY created_at DESC
	LIMIT (COALESCE(NULLIF(@limit_opt :: INT, 0), 25));

-- name: GetFilteredInboxNotificationsByUserID :many
-- Fetches inbox notifications for a user filtered by templates and targets
-- param user_id: The user ID
-- param templates: The template IDs to filter by - the template_id = ANY(@templates::UUID[]) condition checks if the template_id is in the @templates array
-- param targets: The target IDs to filter by - the targets @> COALESCE(@targets, ARRAY[]::UUID[]) condition checks if the targets array (from the DB) contains all the elements in the @targets array
-- param read_status: The read status to filter by - can be any of 'ALL', 'UNREAD', 'READ'
-- param created_at_opt: The created_at timestamp to filter by. This parameter is usd for pagination - it fetches notifications created before the specified timestamp if it is not the zero value
-- param limit_opt: The limit of notifications to fetch. If the limit is not specified, it defaults to 25
SELECT * FROM inbox_notifications WHERE
	user_id = @user_id AND
	(@templates::UUID[] IS NULL OR template_id = ANY(@templates::UUID[])) AND
	(@targets::UUID[] IS NULL OR targets @> @targets::UUID[]) AND
	(@read_status::inbox_notification_read_status = 'all' OR (@read_status::inbox_notification_read_status = 'unread' AND read_at IS NULL) OR (@read_status::inbox_notification_read_status = 'read' AND read_at IS NOT NULL)) AND
	(@created_at_opt::TIMESTAMPTZ = '0001-01-01 00:00:00Z' OR created_at < @created_at_opt::TIMESTAMPTZ)
	ORDER BY created_at DESC
	LIMIT (COALESCE(NULLIF(@limit_opt :: INT, 0), 25));

-- name: GetInboxNotificationByID :one
SELECT * FROM inbox_notifications WHERE id = $1;

-- name: CountUnreadInboxNotificationsByUserID :one
SELECT COUNT(*) FROM inbox_notifications WHERE user_id = $1 AND read_at IS NULL;

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

-- name: UpdateInboxNotificationReadStatus :exec
UPDATE
    inbox_notifications
SET
    read_at = $1
WHERE
    id = $2;
