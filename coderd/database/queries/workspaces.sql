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
	COALESCE(template_name.template_name, 'unknown') as template_name,
	latest_build.template_version_id,
	latest_build.template_version_name,
	COUNT(*) OVER () as count
FROM
    workspaces
JOIN
    users
ON
    workspaces.owner_id = users.id
LEFT JOIN LATERAL (
	SELECT
		workspace_builds.transition,
		workspace_builds.template_version_id,
		template_versions.name AS template_version_name,
		provisioner_jobs.id AS provisioner_job_id,
		provisioner_jobs.started_at,
		provisioner_jobs.updated_at,
		provisioner_jobs.canceled_at,
		provisioner_jobs.completed_at,
		provisioner_jobs.error
	FROM
		workspace_builds
	LEFT JOIN
		provisioner_jobs
	ON
		provisioner_jobs.id = workspace_builds.job_id
	LEFT JOIN
		template_versions
	ON
		template_versions.id = workspace_builds.template_version_id
	WHERE
		workspace_builds.workspace_id = workspaces.id
	ORDER BY
		build_number DESC
	LIMIT
		1
) latest_build ON TRUE
LEFT JOIN LATERAL (
	SELECT
		templates.name AS template_name
	FROM
		templates
	WHERE
		templates.id = workspaces.template_id
) template_name ON true
WHERE
	-- Optionally include deleted workspaces
	workspaces.deleted = @deleted
	AND CASE
		WHEN @status :: text != '' THEN
			CASE
				WHEN @status = 'pending' THEN
					latest_build.started_at IS NULL
				WHEN @status = 'starting' THEN
					latest_build.started_at IS NOT NULL AND
					latest_build.canceled_at IS NULL AND
					latest_build.completed_at IS NULL AND
					latest_build.updated_at - INTERVAL '30 seconds' < NOW() AND
					latest_build.transition = 'start'::workspace_transition

				WHEN @status = 'running' THEN
					latest_build.completed_at IS NOT NULL AND
					latest_build.canceled_at IS NULL AND
					latest_build.error IS NULL AND
					latest_build.transition = 'start'::workspace_transition

				WHEN @status = 'stopping' THEN
					latest_build.started_at IS NOT NULL AND
					latest_build.canceled_at IS NULL AND
					latest_build.completed_at IS NULL AND
					latest_build.updated_at - INTERVAL '30 seconds' < NOW() AND
					latest_build.transition = 'stop'::workspace_transition

				WHEN @status = 'stopped' THEN
					latest_build.completed_at IS NOT NULL AND
					latest_build.canceled_at IS NULL AND
					latest_build.error IS NULL AND
					latest_build.transition = 'stop'::workspace_transition

				WHEN @status = 'failed' THEN
					(latest_build.canceled_at IS NOT NULL AND
						latest_build.error IS NOT NULL) OR
					(latest_build.completed_at IS NOT NULL AND
						latest_build.error IS NOT NULL)

				WHEN @status = 'canceling' THEN
					latest_build.canceled_at IS NOT NULL AND
					latest_build.completed_at IS NULL

				WHEN @status = 'canceled' THEN
					latest_build.canceled_at IS NOT NULL AND
					latest_build.completed_at IS NOT NULL

				WHEN @status = 'deleted' THEN
					latest_build.started_at IS NOT NULL AND
					latest_build.canceled_at IS NULL AND
					latest_build.completed_at IS NOT NULL AND
					latest_build.updated_at - INTERVAL '30 seconds' < NOW() AND
					latest_build.transition = 'delete'::workspace_transition AND
					-- If the error field is not null, the status is 'failed'
					latest_build.error IS NULL

				WHEN @status = 'deleting' THEN
					latest_build.completed_at IS NULL AND
					latest_build.canceled_at IS NULL AND
					latest_build.error IS NULL AND
					latest_build.transition = 'delete'::workspace_transition

				ELSE
					true
			END
		ELSE true
	END
	-- Filter by owner_id
	AND CASE
		WHEN @owner_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN
			workspaces.owner_id = @owner_id
		ELSE true
	END
	-- Filter by owner_name
	AND CASE
		WHEN @owner_username :: text != '' THEN
			workspaces.owner_id = (SELECT id FROM users WHERE lower(username) = lower(@owner_username) AND deleted = false)
		ELSE true
	END
	-- Filter by template_name
	-- There can be more than 1 template with the same name across organizations.
	-- Use the organization filter to restrict to 1 org if needed.
	AND CASE
		WHEN @template_name :: text != '' THEN
			workspaces.template_id = ANY(SELECT id FROM templates WHERE lower(name) = lower(@template_name) AND deleted = false)
		ELSE true
	END
	-- Filter by template_ids
	AND CASE
		WHEN array_length(@template_ids :: uuid[], 1) > 0 THEN
			workspaces.template_id = ANY(@template_ids)
		ELSE true
	END
	-- Filter by name, matching on substring
	AND CASE
		WHEN @name :: text != '' THEN
			workspaces.name ILIKE '%' || @name || '%'
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
	-- Filter by locked workspaces. By default we do not return locked
	-- workspaces since they are considered soft-deleted.
	AND CASE
		WHEN @locked_at :: timestamptz > '0001-01-01 00:00:00+00'::timestamptz THEN
			locked_at IS NOT NULL AND locked_at >= @locked_at
		ELSE
			locked_at IS NULL
	END
	-- Authorize Filter clause will be injected below in GetAuthorizedWorkspaces
	-- @authorize_filter
ORDER BY
	(latest_build.completed_at IS NOT NULL AND
		latest_build.canceled_at IS NULL AND
		latest_build.error IS NULL AND
		latest_build.transition = 'start'::workspace_transition) DESC,
	LOWER(users.username) ASC,
	LOWER(workspaces.name) ASC
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
		ttl,
		last_used_at
	)
VALUES
	($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *;

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

-- name: GetDeploymentWorkspaceStats :one
WITH workspaces_with_jobs AS (
	SELECT
	latest_build.* FROM workspaces
	LEFT JOIN LATERAL (
		SELECT
			workspace_builds.transition,
			provisioner_jobs.id AS provisioner_job_id,
			provisioner_jobs.started_at,
			provisioner_jobs.updated_at,
			provisioner_jobs.canceled_at,
			provisioner_jobs.completed_at,
			provisioner_jobs.error
		FROM
			workspace_builds
		LEFT JOIN
			provisioner_jobs
		ON
			provisioner_jobs.id = workspace_builds.job_id
		WHERE
			workspace_builds.workspace_id = workspaces.id
		ORDER BY
			build_number DESC
		LIMIT
			1
	) latest_build ON TRUE WHERE deleted = false
), pending_workspaces AS (
	SELECT COUNT(*) AS count FROM workspaces_with_jobs WHERE
		started_at IS NULL
), building_workspaces AS (
	SELECT COUNT(*) AS count FROM workspaces_with_jobs WHERE
		started_at IS NOT NULL AND
		canceled_at IS NULL AND
		completed_at IS NULL AND
		updated_at - INTERVAL '30 seconds' < NOW()
), running_workspaces AS (
	SELECT COUNT(*) AS count FROM workspaces_with_jobs WHERE
		completed_at IS NOT NULL AND
		canceled_at IS NULL AND
		error IS NULL AND
		transition = 'start'::workspace_transition
), failed_workspaces AS (
	SELECT COUNT(*) AS count FROM workspaces_with_jobs WHERE
		(canceled_at IS NOT NULL AND
			error IS NOT NULL) OR
		(completed_at IS NOT NULL AND
			error IS NOT NULL)
), stopped_workspaces AS (
	SELECT COUNT(*) AS count FROM workspaces_with_jobs WHERE
		completed_at IS NOT NULL AND
		canceled_at IS NULL AND
		error IS NULL AND
		transition = 'stop'::workspace_transition
)
SELECT
	pending_workspaces.count AS pending_workspaces,
	building_workspaces.count AS building_workspaces,
	running_workspaces.count AS running_workspaces,
	failed_workspaces.count AS failed_workspaces,
	stopped_workspaces.count AS stopped_workspaces
FROM pending_workspaces, building_workspaces, running_workspaces, failed_workspaces, stopped_workspaces;

-- name: GetWorkspacesEligibleForTransition :many
SELECT
	workspaces.*
FROM
	workspaces
LEFT JOIN
	workspace_builds ON workspace_builds.workspace_id = workspaces.id
INNER JOIN
	provisioner_jobs ON workspace_builds.job_id = provisioner_jobs.id
INNER JOIN
	templates ON workspaces.template_id = templates.id
WHERE
	workspace_builds.build_number = (
		SELECT
			MAX(build_number)
		FROM
			workspace_builds
		WHERE
			workspace_builds.workspace_id = workspaces.id
	) AND

	(
		-- If the workspace build was a start transition, the workspace is
		-- potentially eligible for autostop if it's past the deadline. The
		-- deadline is computed at build time upon success and is bumped based
		-- on activity (up the max deadline if set). We don't need to check
		-- license here since that's done when the values are written to the build.
		(
			workspace_builds.transition = 'start'::workspace_transition AND
			workspace_builds.deadline IS NOT NULL AND
			workspace_builds.deadline < @now :: timestamptz
		) OR

		-- If the workspace build was a stop transition, the workspace is
		-- potentially eligible for autostart if it has a schedule set. The
		-- caller must check if the template allows autostart in a license-aware
		-- fashion as we cannot check it here.
		(
			workspace_builds.transition = 'stop'::workspace_transition AND
			workspaces.autostart_schedule IS NOT NULL
		) OR

		-- If the workspace's most recent job resulted in an error
		-- it may be eligible for failed stop.
		(
			provisioner_jobs.error IS NOT NULL AND
			provisioner_jobs.error != '' AND
			workspace_builds.transition = 'start'::workspace_transition
		) OR

		-- If the workspace's template has an inactivity_ttl set
		-- it may be eligible for locking.
		(
			templates.inactivity_ttl > 0 AND
			workspaces.locked_at IS NULL
		) OR

		-- If the workspace's template has a locked_ttl set
		-- and the workspace is already locked
		(
			templates.locked_ttl > 0 AND
			workspaces.locked_at IS NOT NULL
		)
	) AND workspaces.deleted = 'false';

-- name: UpdateWorkspaceLockedDeletingAt :one
UPDATE
	workspaces
SET
	locked_at = $2,
	-- When a workspace is unlocked we want to update the last_used_at to avoid the workspace getting re-locked.
	-- if we're locking the workspace then we leave it alone.
	last_used_at = CASE WHEN $2::timestamptz IS NULL THEN now() at time zone 'utc' ELSE last_used_at END,
	-- If locked_at is null (meaning unlocked) or the template-defined locked_ttl is 0 we should set
	-- deleting_at to NULL else set it to the locked_at + locked_ttl duration.
	deleting_at = CASE WHEN $2::timestamptz IS NULL OR templates.locked_ttl = 0 THEN NULL ELSE $2::timestamptz + INTERVAL '1 milliseconds' * templates.locked_ttl / 1000000 END
FROM
	templates
WHERE
	workspaces.template_id = templates.id
AND
	workspaces.id = $1
RETURNING workspaces.*;

-- name: UpdateWorkspacesLockedDeletingAtByTemplateID :exec
UPDATE workspaces
SET
    deleting_at = CASE
        WHEN @locked_ttl_ms::bigint = 0 THEN NULL
        WHEN @locked_at::timestamptz > '0001-01-01 00:00:00+00'::timestamptz THEN  (@locked_at::timestamptz) + interval '1 milliseconds' * @locked_ttl_ms::bigint
        ELSE locked_at + interval '1 milliseconds' * @locked_ttl_ms::bigint
    END,
    locked_at = CASE WHEN @locked_at::timestamptz > '0001-01-01 00:00:00+00'::timestamptz THEN @locked_at::timestamptz ELSE locked_at END
WHERE
    template_id = @template_id
AND
    locked_at IS NOT NULL;

-- name: UpdateTemplateWorkspacesLastUsedAt :exec
UPDATE workspaces
SET
	last_used_at = @last_used_at::timestamptz
WHERE
	template_id = @template_id;
