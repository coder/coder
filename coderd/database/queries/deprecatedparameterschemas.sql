-- name: GetParameterSchemasByJobID :many
SELECT
	*
FROM
	parameter_schemas
WHERE
	job_id = $1
ORDER BY
	index;

-- name: GetParameterSchemasCreatedAfter :many
SELECT * FROM parameter_schemas WHERE created_at > $1;

-- name: InsertParameterSchema :one
INSERT INTO
	parameter_schemas (
		id,
		created_at,
		job_id,
		"name",
		description,
		default_source_scheme,
		default_source_value,
		allow_override_source,
		default_destination_scheme,
		allow_override_destination,
		default_refresh,
		redisplay_value,
		validation_error,
		validation_condition,
		validation_type_system,
		validation_value_type,
		index
	)
VALUES
	(
		$1,
		$2,
		$3,
		$4,
		$5,
		$6,
		$7,
		$8,
		$9,
		$10,
		$11,
		$12,
		$13,
		$14,
		$15,
		$16,
		$17
	) RETURNING *;
