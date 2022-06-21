-- name: ParameterValue :one
SELECT * FROM
	parameter_values
WHERE
	id = $1;


-- name: DeleteParameterValueByID :exec
DELETE FROM
	parameter_values
WHERE
	id = $1;

-- name: ParameterValues :many
SELECT
	*
FROM
	parameter_values
WHERE
  	CASE
		  WHEN cardinality(@scopes :: parameter_scope[]) > 0 THEN
				  scope = ANY(@scopes :: parameter_scope[])
		  ELSE true
	END
    AND CASE
		WHEN cardinality(@scope_ids :: uuid[]) > 0 THEN
			scope_id = ANY(@scope_ids :: uuid[])
		ELSE true
	END
  	AND CASE
		WHEN cardinality(@ids :: uuid[]) > 0 THEN
			id = ANY(@ids :: uuid[])
		ELSE true
	END
  	AND CASE
		  WHEN cardinality(@names :: text[]) > 0 THEN
				  "name" = ANY(@names :: text[])
		  ELSE true
	END
;

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
