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
    users.avatar_url AS user_avatar_url,
    COUNT(audit_logs.*) OVER () AS count
FROM
    audit_logs
    LEFT JOIN users ON audit_logs.user_id = users.id
    LEFT JOIN
        -- First join on workspaces to get the initial workspace create
        -- to workspace build 1 id. This is because the first create is
        -- is a different audit log than subsequent starts.
        workspaces ON
		    audit_logs.resource_type = 'workspace' AND
			audit_logs.resource_id = workspaces.id
    LEFT JOIN
	    workspace_builds ON
            -- Get the reason from the build if the resource type
            -- is a workspace_build
            (
			    audit_logs.resource_type = 'workspace_build'
                AND audit_logs.resource_id = workspace_builds.id
			)
            OR
            -- Get the reason from the build #1 if this is the first
            -- workspace create.
            (
				audit_logs.resource_type = 'workspace' AND
				audit_logs.action = 'create' AND
				workspaces.id = workspace_builds.workspace_id AND
				workspace_builds.build_number = 1
			)
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
		WHEN @date_from :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			"time" >= @date_from
		ELSE true
	END
	-- Filter by date_to
	AND CASE
		WHEN @date_to :: timestamp with time zone != '0001-01-01 00:00:00Z' THEN
			"time" <= @date_to
		ELSE true
	END
    -- Filter by build_reason
    AND CASE
	    WHEN @build_reason::text != '' THEN
            workspace_builds.reason::text = @build_reason
        ELSE true
    END
ORDER BY
    "time" DESC
LIMIT
    $1
OFFSET
    $2;

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
