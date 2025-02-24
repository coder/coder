-- name: GetInboxNotificationsByUserID :many
SELECT * FROM inbox_notifications WHERE
	user_id = @user_id AND
	(@read_status::text = 'ALL' OR (@read_status::text = 'UNREAD' AND read_at IS NULL) OR (@read_status::text = 'READ' AND read_at IS NOT NULL)) AND
	(@created_at_opt::TIMESTAMPTZ = '0001-01-01 00:00:00Z' OR created_at < @created_at_opt::TIMESTAMPTZ)
	ORDER BY created_at DESC
	LIMIT (COALESCE(NULLIF(@limit_opt :: INT, 0), 25));

-- name: GetInboxNotificationsByUserIDFilteredByTemplatesAndTargets :many
-- Fetches inbox notifications for a user filtered by templates and targets
-- param user_id: The user ID
-- param templates: The template IDs to filter by - the template_id = ANY(@templates::UUID[]) condition checks if the template_id is in the @templates array
-- param targets: The target IDs to filter by - the targets @> COALESCE(@targets, ARRAY[]::UUID[]) condition checks if the targets array (from the DB) contains all the elements in the @targets array
SELECT * FROM inbox_notifications WHERE
	user_id = @user_id AND
	template_id = ANY(@templates::UUID[]) AND
	targets @> COALESCE(@targets, ARRAY[]::UUID[]) AND
	(@read_status::text = 'ALL' OR (@read_status::text = 'UNREAD' AND read_at IS NULL) OR (@read_status::text = 'READ' AND read_at IS NOT NULL)) AND
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
