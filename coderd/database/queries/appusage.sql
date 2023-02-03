-- name: InsertAppUsage :one
INSERT INTO
	app_usage (
		user_id,
		template_id,
		app_id,
		created_at
	)
VALUES
	($1, $2, $3, $4) RETURNING *;

-- name: GetAppUsageByDate :one
SELECT
	*
FROM
	app_usage
WHERE
	user_id = $1 AND template_id = $2 AND app_id = $3 AND created_at = $4;

-- name: GetAppUsageByTemplateID :many
SELECT
	*
FROM
	app_usage
WHERE
	template_id = $1 AND created_at >= @since_date :: date AND created_at <= @to_date :: date
ORDER BY
	created_at ASC;

-- name: GetGroupedAppUsageByTemplateID :many
SELECT
	app_usage.created_at,
	app_usage.app_id,
	workspace_apps.display_name as app_display_name,
	workspace_apps.icon as app_icon,
	COUNT(*)
FROM
	app_usage
JOIN
	workspace_apps
ON
	app_usage.app_id = workspace_apps.id
WHERE
	app_usage.template_id = $1 AND app_usage.created_at BETWEEN @since_date :: date AND @to_date :: date
GROUP BY
	app_usage.created_at, app_usage.app_id
ORDER BY
	app_usage.created_at ASC;
