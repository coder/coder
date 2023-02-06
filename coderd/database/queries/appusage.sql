-- name: InsertAppUsage :exec
INSERT INTO
	app_usage (
		user_id,
		template_id,
		app_slug,
		created_at
	)
VALUES
	($1, $2, $3, $4)
ON CONFLICT (user_id, app_slug, template_id, created_at) DO NOTHING;

-- name: GetAppUsageByDate :one
SELECT
	*
FROM
	app_usage
WHERE
	user_id = $1 AND template_id = $2 AND app_slug = $3 AND created_at = $4;

-- name: GetAppUsageByTemplateID :many
SELECT
	app_usage.created_at,
	app_usage.app_slug,
	workspace_apps.display_name as app_display_name,
	workspace_apps.icon as app_icon,
  COUNT(*)
FROM
  app_usage
JOIN
	workspace_apps
ON
  app_usage.app_slug = workspace_apps.slug
WHERE
  app_usage.template_id = $1 AND app_usage.created_at BETWEEN @since_date :: date AND @to_date :: date
GROUP BY
  app_usage.created_at,
  app_usage.app_slug,
  workspace_apps.display_name,
  workspace_apps.icon
ORDER BY
  app_usage.created_at ASC;
