-- name: DeleteParameterValueByID :exec
DELETE FROM
	parameter_values
WHERE
	id = $1;

-- name: GetParameterValuesByScope :many
SELECT
	*
FROM
	parameter_values
WHERE
	scope = $1
	AND scope_id = $2;

-- name: GetParameterValueByScopeAndName :one
SELECT
	*
FROM
	parameter_values
WHERE
	scope = $1
	AND scope_id = $2
	AND NAME = $3
LIMIT
	1;

-- name: InsertParameterValue :one
INSERT INTO
	parameter_values (
		id,
		"name",
		created_at,
		updated_at,
		scope,
		scope_id,
		source_scheme,
		source_value,
		destination_scheme
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;
