-- name: GetParameterSchemasByJobID :many
SELECT
	*
FROM
	parameter_schemas
WHERE
	job_id = $1
ORDER BY
	index;
