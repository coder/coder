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
