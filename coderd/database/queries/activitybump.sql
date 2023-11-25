-- Bumps the workspace deadline by the TTL of the workspace template (or the
-- template's default_ttl if the template doesn't allow_user_autostop), OR by 1h
-- if the template has activity_bump_by_1h set to true.
--
-- If the workspace bump will cross an autostart threshold, then the bump is
-- autostart + TTL. This is the deadline behavior if the workspace was to
-- autostart from a stopped state.
--
-- Max deadline is respected, and will never be bumped. The deadline will never
-- decrease.
--
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
		) AS ttl_interval_base,
		(
			CASE
				WHEN templates.activity_bump_by_1h
				THEN ('1 hour')::interval
				ELSE ttl_interval_base
			END
		) AS ttl_interval,
		(
			CASE
				-- If the extension would push us over the next_autostart
				-- interval, then extend the deadline by the full ttl from
				-- the autostart time. This will essentially be as if the
				-- workspace auto started at the given time and the original
				-- TTL was applied.
				WHEN NOW() + ttl_interval > @next_autostart :: timestamptz
				    -- If the autostart is behind now(), then the
					-- autostart schedule is either the 0 time and not provided,
					-- or it was the autostart in the past, which is no longer
					-- relevant. If autostart is > 0 and in the past, then
					-- that is a mistake by the caller.
					AND @next_autostart > NOW()
					THEN
					-- Extend to the autostart, then add the original TTL
					((@next_autostart :: timestamptz) - NOW()) + ttl_interval_base
				ELSE ttl_interval
			END
		) AS extended_ttl_interval
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
		THEN GREATEST(wb.deadline, NOW() + l.extended_ttl_interval)
		ELSE LEAST(GREATEST(wb.deadline, NOW() + l.extended_ttl_interval), l.build_max_deadline)
	END
FROM latest l
WHERE wb.id = l.build_id
AND l.job_completed_at IS NOT NULL
AND l.build_transition = 'start'
-- We only bump if the raw interval is positive and non-zero.
AND l.ttl_interval_base > '0 seconds'::interval
-- We only bump if workspace shutdown is manual.
AND l.build_deadline != '0001-01-01 00:00:00+00'
-- We only bump when 5% of the deadline has elapsed.
AND l.build_deadline - (l.extended_ttl_interval * 0.95) < NOW()
;
