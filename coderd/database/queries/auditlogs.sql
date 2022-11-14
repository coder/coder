-- GetAuditLogsBefore retrieves `row_limit` number of audit logs before the provided
-- ID.
-- name: GetAuditLogsOffset :many
SELECT
	audit_logs.*,
    users.username AS user_username,
    users.email AS user_email,
    users.created_at AS user_created_at,
    users.status AS user_status,
    users.rbac_roles AS user_roles,
    users.avatar_url AS user_avatar_url
FROM
	audit_logs
LEFT JOIN
    users ON audit_logs.user_id = users.id
WHERE
    -- Filter resource_type
	CASE
		WHEN @resource_type :: text != '' THEN
			resource_type = @resource_type :: resource_type
		ELSE true
	END
	-- Filter resource_id
	AND CASE
		WHEN @resource_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			resource_id = @resource_id
		ELSE true
	END
	-- Filter by resource_target
	AND CASE
		WHEN @resource_target :: text != '' THEN
			resource_target = @resource_target
		ELSE true
	END
	-- Filter action
	AND CASE
		WHEN @action :: text != '' THEN
			action = @action :: audit_action
		ELSE true
	END
	-- Filter by username
	AND CASE
		WHEN @username :: text != '' THEN
			users.username = @username
		ELSE true
	END
	-- Filter by user_email
	AND CASE
		WHEN @email :: text != '' THEN
			users.email = @email
		ELSE true
	END
	-- Filter by date_from
	AND CASE
		WHEN @date_from :: timestamp with time zone != '0001-01-01 00:00:00' THEN
			"time" >= @date_from
		ELSE true
	END
	-- Filter by date_to
	AND CASE
		WHEN @date_to :: timestamp with time zone != '0001-01-01 00:00:00' THEN
			"time" <= @date_to
		ELSE true
	END
ORDER BY
    "time" DESC
LIMIT
    $1
OFFSET
    $2;

-- name: GetAuditLogCount :one
SELECT
  COUNT(*) as count
FROM
	audit_logs
WHERE
    -- Filter resource_type
	CASE
		WHEN @resource_type :: text != '' THEN
			resource_type = @resource_type :: resource_type
		ELSE true
	END
	-- Filter resource_id
	AND CASE
		WHEN @resource_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			resource_id = @resource_id
		ELSE true
	END
	-- Filter by resource_target
	AND CASE
		WHEN @resource_target :: text != '' THEN
			resource_target = @resource_target
		ELSE true
	END
	-- Filter action
	AND CASE
		WHEN @action :: text != '' THEN
			action = @action :: audit_action
		ELSE true
	END
	-- Filter by username
	AND CASE
		WHEN @username :: text != '' THEN
			user_id = (SELECT id from users WHERE users.username = @username )
		ELSE true
	END
	-- Filter by user_email
	AND CASE
		WHEN @email :: text != '' THEN
			user_id = (SELECT id from users WHERE users.email = @email )
		ELSE true
	END
	-- Filter by date_from
	AND CASE
		WHEN @date_from :: timestamp with time zone != '0001-01-01 00:00:00' THEN
			"time" >= @date_from
		ELSE true
	END
	-- Filter by date_to
	AND CASE
		WHEN @date_to :: timestamp with time zone != '0001-01-01 00:00:00' THEN
			"time" <= @date_to
		ELSE true
	END;

-- name: InsertAuditLog :one
INSERT INTO
	audit_logs (
        id,
        "time",
        user_id,
        organization_id,
        ip,
        user_agent,
        resource_type,
        resource_id,
        resource_target,
        action,
        diff,
        status_code,
        additional_fields,
        request_id,
        resource_icon
    )
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) RETURNING *;
