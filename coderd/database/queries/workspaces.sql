-- name: GetWorkspaceByID :one
SELECT
	*
FROM
	workspaces
WHERE
	id = $1
LIMIT
	1;

-- name: GetWorkspaceByWorkspaceAppID :one
SELECT
	*
FROM
	workspaces
WHERE
		workspaces.id = (
		SELECT
			workspace_id
		FROM
			workspace_builds
		WHERE
				workspace_builds.job_id = (
				SELECT
					job_id
				FROM
					workspace_resources
				WHERE
						workspace_resources.id = (
						SELECT
							resource_id
						FROM
							workspace_agents
						WHERE
								workspace_agents.id = (
								SELECT
									agent_id
								FROM
									workspace_apps
								WHERE
									workspace_apps.id = @workspace_app_id
								)
					)
			)
	);

-- name: GetWorkspaceByAgentID :one
SELECT
	*
FROM
	workspaces
WHERE
	workspaces.id = (
		SELECT
			workspace_id
		FROM
			workspace_builds
		WHERE
			workspace_builds.job_id = (
				SELECT
					job_id
				FROM
					workspace_resources
				WHERE
					workspace_resources.id = (
						SELECT
							resource_id
						FROM
							workspace_agents
						WHERE
							workspace_agents.id = @agent_id
					)
			)
	);

-- name: GetWorkspaces :many
SELECT
	workspaces.*,
	-- Template fields that are readable from a workspace
	-- These values should never be null. This is required for sqlc to not
	-- generate nullable golang fields.
	COALESCE(templates.name, '') AS template_name,
	COALESCE(templates.icon, '') AS template_icon,
	COALESCE(templates.display_name, 'unknown') AS template_display_name,
	COALESCE(templates.allow_user_cancel_workspace_jobs, false) AS template_allow_user_cancel,
	COALESCE(templates.active_version_id, '00000000-0000-0000-0000-000000000000'::uuid) AS template_active_version_id,
	-- Workspace build fields
	COALESCE(latest_build.id, '00000000-0000-0000-0000-000000000000'::uuid) AS latest_build_id,
	COALESCE(latest_build.created_at, '0001-01-01 00:00:00+00:00') AS latest_build_created_at,
	COALESCE(latest_build.updated_at, '0001-01-01 00:00:00+00:00') AS latest_build_updated_at,
	COALESCE(latest_build.template_version_id, '00000000-0000-0000-0000-000000000000'::uuid) AS latest_build_template_version_id,
	COALESCE(latest_build.build_number, -1) AS latest_build_number,
	COALESCE(latest_build.transition, 'start') AS latest_build_transition,
	COALESCE(latest_build.job_id, '00000000-0000-0000-0000-000000000000'::uuid) AS latest_build_job_id,
	COALESCE(latest_build.initiator_id, '00000000-0000-0000-0000-000000000000'::uuid) AS latest_build_initiator_id,
	COALESCE(latest_build.reason, 'initiator') AS latest_build_reason,
	COALESCE(latest_build.daily_cost, 0) AS latest_build_daily_cost,
	-- Provisioner job fields
	COALESCE(latest_build_job.id, '00000000-0000-0000-0000-000000000000'::uuid) AS latest_build_job_id,
	COALESCE(latest_build_job.created_at, '0001-01-01 00:00:00+00:00') AS latest_build_job_created_at,
	COALESCE(latest_build_job.updated_at, '0001-01-01 00:00:00+00:00') AS latest_build_job_updated_at,
	latest_build_job.started_at AS latest_build_job_started_at,
	latest_build_job.completed_at AS latest_build_job_completed_at,
	latest_build_job.canceled_at AS latest_build_job_canceled_at,
	COALESCE(latest_build_job.error, 'job not found') AS latest_build_job_error,
	COALESCE(latest_build_job.organization_id, '00000000-0000-0000-0000-000000000000'::uuid) AS latest_build_job_organization_id,
	COALESCE(latest_build_job.provisioner, 'echo') AS latest_build_job_provisioner,
	COALESCE(latest_build_job.storage_method, 'file') AS latest_build_job_storage_method,
	COALESCE(latest_build_job.type, 'workspace_build') AS latest_build_job_type,
	COALESCE(latest_build_job.worker_id, '00000000-0000-0000-0000-000000000000'::uuid) AS latest_build_job_worker_id,
	COALESCE(latest_build_job.file_id, '00000000-0000-0000-0000-000000000000'::uuid) AS latest_build_job_file_id,
	COALESCE(latest_build_job.tags, '{}') AS latest_build_job_tags,

	-- For pagination
	COUNT(*) OVER () as count
FROM
	workspaces
		LEFT JOIN
	templates ON templates.id = workspaces.template_id
		LEFT JOIN LATERAL
		( -- Join the latest workspace build
		SELECT
			*
		FROM
			workspace_builds
		WHERE
			workspaces.id = workspace_builds.workspace_id
		ORDER BY
			build_number DESC
			FETCH FIRST 1 ROW ONLY
		) latest_build
			ON true
		LEFT JOIN
	provisioner_jobs latest_build_job ON latest_build.job_id = latest_build_job.id
WHERE
	-- Optionally include deleted workspaces
	workspaces.deleted = @deleted
	AND CASE
		WHEN @status :: text != '' THEN
			CASE
				WHEN @status = 'pending' THEN
					latest_build_job.started_at IS NULL
				WHEN @status = 'starting' THEN
					latest_build_job.started_at IS NOT NULL AND
					latest_build_job.canceled_at IS NULL AND
					latest_build_job.completed_at IS NULL AND
					latest_build.updated_at - INTERVAL '30 seconds' < NOW() AND
					latest_build.transition = 'start'::workspace_transition

				WHEN @status = 'running' THEN
					latest_build_job.completed_at IS NOT NULL AND
					latest_build_job.canceled_at IS NULL AND
					latest_build_job.error IS NULL AND
					latest_build.transition = 'start'::workspace_transition

				WHEN @status = 'stopping' THEN
					latest_build_job.error IS NOT NULL AND
					latest_build_job.canceled_at IS NULL AND
					latest_build_job.completed_at IS NULL AND
					latest_build.updated_at - INTERVAL '30 seconds' < NOW() AND
					latest_build.transition = 'stop'::workspace_transition

				WHEN @status = 'stopped' THEN
					latest_build_job.completed_at IS NOT NULL AND
					latest_build_job.canceled_at IS NULL AND
					latest_build_job.error IS NULL AND
					latest_build.transition = 'stop'::workspace_transition

				WHEN @status = 'failed' THEN
					(latest_build_job.canceled_at IS NOT NULL AND
					 latest_build_job.error IS NOT NULL) OR
					(latest_build_job.completed_at IS NOT NULL AND
					 latest_build_job.error IS NOT NULL)

				WHEN @status = 'canceling' THEN
					latest_build_job.canceled_at IS NOT NULL AND
					latest_build_job.completed_at IS NULL

				WHEN @status = 'canceled' THEN
					latest_build_job.canceled_at IS NOT NULL AND
					latest_build_job.completed_at IS NOT NULL

				WHEN @status = 'deleted' THEN
					latest_build_job.error IS NOT NULL AND
					latest_build_job.canceled_at IS NULL AND
					latest_build_job.completed_at IS NOT NULL AND
					latest_build.updated_at - INTERVAL '30 seconds' < NOW() AND
					latest_build.transition = 'delete'::workspace_transition

				WHEN @status = 'deleting' THEN
					latest_build_job.completed_at IS NOT NULL AND
					latest_build_job.canceled_at IS NULL AND
					latest_build_job.error IS NULL AND
					latest_build.transition = 'delete'::workspace_transition
				ELSE
					true
				END
		ELSE true
	END
	-- Filter by owner_id
	AND CASE
		WHEN @owner_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			owner_id = @owner_id
		ELSE true
	END
	-- Filter by owner_name
	AND CASE
		WHEN @owner_username :: text != '' THEN
			owner_id = (SELECT id FROM users WHERE lower(username) = lower(@owner_username) AND deleted = false)
		ELSE true
	END
	-- Filter by template_name
	-- There can be more than 1 template with the same name across organizations.
	-- Use the organization filter to restrict to 1 org if needed.
	AND CASE
		WHEN @template_name :: text != '' THEN
				lower(templates.name) = lower(@template_name)
		ELSE true
	END
	-- Filter by template_ids
	AND CASE
		WHEN array_length(@template_ids :: uuid[], 1) > 0 THEN
			template_id = ANY(@template_ids)
		ELSE true
	END
	-- Filter by workspace_ids
	AND CASE
		WHEN array_length(@workspace_ids :: uuid[], 1) > 0 THEN
			id = ANY(@workspace_ids)
		ELSE true
	END
	-- Filter by name, matching on substring
	AND CASE
		WHEN @name :: text != '' THEN
			name ILIKE '%' || @name || '%'
		ELSE true
	END
	-- Filter by agent status
	-- has-agent: is only applicable for workspaces in "start" transition. Stopped and deleted workspaces don't have agents.
	AND CASE
		WHEN @has_agent :: text != '' THEN
			(
				SELECT COUNT(*)
				FROM
					workspace_resources
						JOIN
					workspace_agents
					ON
						workspace_agents.resource_id = workspace_resources.id
				WHERE
					workspace_resources.job_id = latest_build.provisioner_job_id AND
					latest_build.transition = 'start'::workspace_transition AND
					@has_agent = (
					CASE
						WHEN workspace_agents.first_connected_at IS NULL THEN
							CASE
								WHEN workspace_agents.connection_timeout_seconds > 0 AND NOW() - workspace_agents.created_at > workspace_agents.connection_timeout_seconds * INTERVAL '1 second' THEN
									'timeout'
								ELSE
									'connecting'
								END
						WHEN workspace_agents.disconnected_at > workspace_agents.last_connected_at THEN
							'disconnected'
						WHEN NOW() - workspace_agents.last_connected_at > INTERVAL '1 second' * @agent_inactive_disconnect_timeout_seconds :: bigint THEN
							'disconnected'
						WHEN workspace_agents.last_connected_at IS NOT NULL THEN
							'connected'
						ELSE
							NULL
						END
					)
			) > 0
		ELSE true
	END
	-- Authorize Filter clause will be injected below in GetAuthorizedWorkspaces
	-- @authorize_filter
ORDER BY
	last_used_at DESC
LIMIT
	CASE
		WHEN @limit_ :: integer > 0 THEN
			@limit_
	END
OFFSET
	@offset_
;

-- name: GetWorkspaceByOwnerIDAndName :one
SELECT
	*
FROM
	workspaces
WHERE
	owner_id = @owner_id
	AND deleted = @deleted
	AND LOWER("name") = LOWER(@name)
ORDER BY created_at DESC;

-- name: InsertWorkspace :one
INSERT INTO
	workspaces (
		id,
		created_at,
		updated_at,
		owner_id,
		organization_id,
		template_id,
		name,
		autostart_schedule,
		ttl
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: UpdateWorkspaceDeletedByID :exec
UPDATE
	workspaces
SET
	deleted = $2
WHERE
	id = $1;

-- name: UpdateWorkspace :one
UPDATE
	workspaces
SET
	name = $2
WHERE
	id = $1
	AND deleted = false
RETURNING *;

-- name: UpdateWorkspaceAutostart :exec
UPDATE
	workspaces
SET
	autostart_schedule = $2
WHERE
	id = $1;

-- name: UpdateWorkspaceTTL :exec
UPDATE
	workspaces
SET
	ttl = $2
WHERE
	id = $1;

-- name: UpdateWorkspaceLastUsedAt :exec
UPDATE
	workspaces
SET
	last_used_at = $2
WHERE
	id = $1;
