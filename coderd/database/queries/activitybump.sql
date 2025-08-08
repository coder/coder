-- Bumps the workspace deadline by the template's configured "activity_bump"
-- duration (default 1h). If the workspace bump will cross an autostart
-- threshold, then the bump is autostart + TTL. This is the deadline behavior if
-- the workspace was to autostart from a stopped state.
--
-- Max deadline is respected, and the deadline will never be bumped past it.
-- The deadline will never decrease.
-- name: ActivityBumpWorkspace :exec
WITH latest AS (
	SELECT
		workspace_builds.id::uuid AS build_id,
		workspace_builds.deadline::timestamp with time zone AS build_deadline,
		workspace_builds.max_deadline::timestamp with time zone AS build_max_deadline,
		workspace_builds.transition AS build_transition,
		provisioner_jobs.completed_at::timestamp with time zone AS job_completed_at,
		templates.activity_bump AS activity_bump,
		(
			CASE
				-- If the extension would push us over the next_autostart
				-- interval, then extend the deadline by the full TTL (NOT
				-- activity bump) from the autostart time. This will essentially
				-- be as if the workspace auto started at the given time and the
				-- original TTL was applied.
				--
				-- Sadly we can't define `activity_bump_interval` above since
				-- it won't be available for this CASE statement, so we have to
				-- copy the cast twice.
				WHEN NOW() + (templates.activity_bump / 1000 / 1000 / 1000 || ' seconds')::interval > @next_autostart :: timestamptz
				    -- If the autostart is behind now(), then the
					-- autostart schedule is either the 0 time and not provided,
					-- or it was the autostart in the past, which is no longer
					-- relevant. If autostart is > 0 and in the past, then
					-- that is a mistake by the caller.
					AND @next_autostart > NOW()
					THEN
					-- Extend to the autostart, then add the activity bump
					((@next_autostart :: timestamptz) - NOW()) + CASE
						WHEN templates.allow_user_autostop
					    	THEN (workspaces.ttl / 1000 / 1000 / 1000 || ' seconds')::interval
							ELSE (templates.default_ttl / 1000 / 1000 / 1000 || ' seconds')::interval
					END

				-- Default to the activity bump duration.
				ELSE
					(templates.activity_bump / 1000 / 1000 / 1000 || ' seconds')::interval
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
		-- Never reduce the deadline from activity.
		THEN GREATEST(wb.deadline, NOW() + l.ttl_interval)
		ELSE LEAST(GREATEST(wb.deadline, NOW() + l.ttl_interval), l.build_max_deadline)
	END
FROM latest l
WHERE wb.id = l.build_id
AND l.job_completed_at IS NOT NULL
-- We only bump if the template has an activity bump duration set.
AND l.activity_bump > 0
AND l.build_transition = 'start'
-- We only bump if the raw interval is positive and non-zero.
AND l.ttl_interval > '0 seconds'::interval
-- We only bump if workspace shutdown is manual.
AND l.build_deadline != '0001-01-01 00:00:00+00'
-- We only bump when 5% of the deadline has elapsed.
AND l.build_deadline - (l.ttl_interval * 0.95) < NOW()
;
