-- We bump by the original TTL to prevent counter-intuitive behavior
-- as the TTL wraps. For example, if I set the TTL to 12 hours, sign off
-- work at midnight, come back at 10am, I would want another full day
-- of uptime.
-- name: ActivityBumpWorkspace :exec
WITH latest AS (
	SELECT
		workspace_builds.id::uuid AS build_id,
		workspace_builds.deadline::timestamp with time zone AS build_deadline,
		workspace_builds.max_deadline::timestamp with time zone AS build_max_deadline,
		workspace_builds.transition AS build_transition,
		provisioner_jobs.completed_at::timestamp with time zone AS job_completed_at,
		(
			CASE
				WHEN templates.allow_user_autostop
				THEN (workspaces.ttl / 1000 / 1000 / 1000 || ' seconds')::interval
				ELSE (templates.default_ttl / 1000 / 1000 / 1000 || ' seconds')::interval
			END
		) AS ttl_interval
	FROM workspace_builds
	JOIN provisioner_jobs
		ON provisioner_jobs.id = workspace_builds.job_id
	JOIN workspaces
		ON workspaces.id = workspace_builds.workspace_id
	JOIN templates
		ON templates.id = workspaces.template_id
	WHERE workspace_builds.workspace_id = @workspace_id::uuid
	ORDER BY workspace_builds.build_number DESC
	LIMIT 1
)
UPDATE
	workspace_builds wb
SET
	updated_at = NOW(),
	deadline = CASE
		WHEN l.build_max_deadline = '0001-01-01 00:00:00+00'
		THEN NOW() + l.ttl_interval
		ELSE LEAST(NOW() + l.ttl_interval, l.build_max_deadline)
	END
FROM latest l
WHERE wb.id = l.build_id
AND l.job_completed_at IS NOT NULL
AND l.build_transition = 'start'
-- We only bump if the raw interval is positive and non-zero.
AND l.ttl_interval > '0 seconds'::interval
-- We only bump if workspace shutdown is manual.
AND l.build_deadline != '0001-01-01 00:00:00+00'
-- We only bump when 5% of the deadline has elapsed.
AND l.build_deadline - (l.ttl_interval * 0.95) < NOW()
;
