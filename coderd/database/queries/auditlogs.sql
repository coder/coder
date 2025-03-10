-- GetAuditLogsBefore retrieves `row_limit` number of audit logs before the provided
-- ID.
-- name: GetAuditLogsOffset :many
SELECT
    sqlc.embed(audit_logs),
    -- sqlc.embed(users) would be nice but it does not seem to play well with
    -- left joins.
    users.username AS user_username,
    users.name AS user_name,
    users.email AS user_email,
    users.created_at AS user_created_at,
    users.updated_at AS user_updated_at,
    users.last_seen_at AS user_last_seen_at,
    users.status AS user_status,
    users.login_type AS user_login_type,
    users.rbac_roles AS user_roles,
    users.avatar_url AS user_avatar_url,
    users.deleted AS user_deleted,
    users.quiet_hours_schedule AS user_quiet_hours_schedule,
    COALESCE(organizations.name, '') AS organization_name,
    COALESCE(organizations.display_name, '') AS organization_display_name,
    COALESCE(organizations.icon, '') AS organization_icon,
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
		LEFT JOIN organizations ON audit_logs.organization_id = organizations.id
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
  	-- Filter organization_id
  	AND CASE
		WHEN @organization_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			audit_logs.organization_id = @organization_id
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
	-- Filter by user_id
	AND CASE
		WHEN @user_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			user_id = @user_id
		ELSE true
	END
	-- Filter by username
	AND CASE
		WHEN @username :: text != '' THEN
			user_id = (SELECT id FROM users WHERE lower(username) = lower(@username) AND deleted = false)
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
	-- Filter request_id
	AND CASE
		WHEN @request_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			audit_logs.request_id = @request_id
		ELSE true
	END

	-- Authorize Filter clause will be injected below in GetAuthorizedAuditLogsOffset
	-- @authorize_filter
ORDER BY
    "time" DESC
LIMIT
	-- a limit of 0 means "no limit". The audit log table is unbounded
	-- in size, and is expected to be quite large. Implement a default
	-- limit of 100 to prevent accidental excessively large queries.
	COALESCE(NULLIF(@limit_opt :: int, 0), 100)
OFFSET
    @offset_opt;

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
