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

-- name: GetAppUsageByTemplateID :many
SELECT
	app_usage.created_at,
	app_usage.app_slug,
  COUNT(*)
FROM
  app_usage
WHERE
  app_usage.template_id = $1 AND app_usage.created_at BETWEEN @since_date :: date AND @until_date :: date
GROUP BY
  app_usage.created_at,
  app_usage.app_slug
ORDER BY
  app_usage.created_at ASC;
