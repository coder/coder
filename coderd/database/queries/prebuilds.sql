-- name: GetRunningPrebuilds :many
SELECT p.id               AS workspace_id,
	   p.name             AS workspace_name,
	   p.template_id,
	   b.template_version_id,
	   tvp_curr.id        AS current_preset_id,
	   -- TODO: just because a prebuild is in a ready state doesn't mean it's eligible; if the prebuild is due to be
	   --       deleted to reconcile state then it MUST NOT be eligible for claiming. We'll need some kind of lock here.
	   CASE
		   WHEN p.lifecycle_state = 'ready'::workspace_agent_lifecycle_state THEN TRUE
		   ELSE FALSE END AS ready,
	   p.created_at
FROM workspace_prebuilds p
		 INNER JOIN workspace_latest_build b ON b.workspace_id = p.id
		 INNER JOIN provisioner_jobs pj ON b.job_id = pj.id
		 INNER JOIN templates t ON p.template_id = t.id
		 LEFT JOIN template_version_presets tvp_curr
				   ON tvp_curr.id = p.current_preset_id -- See https://github.com/coder/internal/issues/398.
WHERE (b.transition = 'start'::workspace_transition
	-- Jobs that are not in terminal states.
	AND pj.job_status = 'succeeded'::provisioner_job_status);

-- name: GetTemplatePresetsWithPrebuilds :many
SELECT t.id                        AS template_id,
	   tv.id                       AS template_version_id,
	   tv.id = t.active_version_id AS using_active_version,
	   tvpp.preset_id,
	   tvp.name,
	   tvpp.desired_instances      AS desired_instances,
	   t.deleted,
	   t.deprecated != ''          AS deprecated
FROM templates t
		 INNER JOIN template_versions tv ON tv.template_id = t.id
		 INNER JOIN template_version_presets tvp ON tvp.template_version_id = tv.id
		 INNER JOIN template_version_preset_prebuilds tvpp ON tvpp.preset_id = tvp.id
WHERE (t.id = sqlc.narg('template_id')::uuid OR sqlc.narg('template_id') IS NULL);

-- name: GetPrebuildsInProgress :many
SELECT t.id AS template_id, wpb.template_version_id, wpb.transition, COUNT(wpb.transition)::int AS count
FROM workspace_latest_build wlb
		 INNER JOIN provisioner_jobs pj ON wlb.job_id = pj.id
		 INNER JOIN workspace_prebuild_builds wpb ON wpb.id = wlb.id
		 INNER JOIN templates t ON t.active_version_id = wlb.template_version_id
WHERE pj.job_status IN ('pending'::provisioner_job_status, 'running'::provisioner_job_status)
GROUP BY t.id, wpb.template_version_id, wpb.transition;

-- name: GetPresetsBackoff :many
WITH filtered_builds AS (
	-- Only select builds which are for prebuild creations
	SELECT wlb.*, tvp.id AS preset_id, pj.job_status, tvpp.desired_instances
	FROM template_version_presets tvp
			 JOIN workspace_latest_build wlb ON wlb.template_version_preset_id = tvp.id
			 JOIN provisioner_jobs pj ON wlb.job_id = pj.id
			 JOIN template_versions tv ON wlb.template_version_id = tv.id
			 JOIN templates t ON tv.template_id = t.id AND t.active_version_id = tv.id
			 JOIN template_version_preset_prebuilds tvpp ON tvpp.preset_id = tvp.id
	WHERE wlb.transition = 'start'::workspace_transition),
	 latest_builds AS (
		 -- Select only the latest build per template_version AND preset
		 SELECT fb.*,
				ROW_NUMBER() OVER (PARTITION BY fb.template_version_preset_id ORDER BY fb.created_at DESC) as rn
		 FROM filtered_builds fb),
	 failed_count AS (
		 -- Count failed builds per template version/preset in the given period
		 SELECT preset_id, COUNT(*) AS num_failed
		 FROM filtered_builds
		 WHERE job_status = 'failed'::provisioner_job_status
		   AND created_at >= @lookback::timestamptz
		 GROUP BY preset_id)
SELECT lb.template_version_id,
	   lb.preset_id,
	   MAX(lb.job_status)::provisioner_job_status AS latest_build_status,
	   MAX(COALESCE(fc.num_failed, 0))::int       AS num_failed,
	   MAX(lb.created_at)::timestamptz            AS last_build_at
FROM latest_builds lb
		 LEFT JOIN failed_count fc ON fc.preset_id = lb.preset_id
WHERE lb.rn <= lb.desired_instances -- Fetch the last N builds, where N is the number of desired instances; if any fail, we backoff
  AND lb.job_status = 'failed'::provisioner_job_status
GROUP BY lb.template_version_id, lb.preset_id, lb.job_status;

-- name: ClaimPrebuild :one
-- TODO: rewrite to use named CTE instead?
UPDATE workspaces w
SET owner_id   = @new_user_id::uuid,
	name       = @new_name::text,
	updated_at = NOW()
WHERE w.id IN (SELECT p.id
			   FROM workspace_prebuilds p
						INNER JOIN workspace_latest_build b ON b.workspace_id = p.id
						INNER JOIN provisioner_jobs pj ON b.job_id = pj.id
						INNER JOIN templates t ON p.template_id = t.id
			   WHERE (b.transition = 'start'::workspace_transition
				   AND pj.job_status IN ('succeeded'::provisioner_job_status))
				 AND b.template_version_id = t.active_version_id
				 AND b.template_version_preset_id = @preset_id::uuid
				 AND p.lifecycle_state = 'ready'::workspace_agent_lifecycle_state
			   ORDER BY random()
			   LIMIT 1 FOR UPDATE OF p SKIP LOCKED)
RETURNING w.id, w.name;

-- name: InsertPresetPrebuild :one
INSERT INTO template_version_preset_prebuilds (id, preset_id, desired_instances, invalidate_after_secs)
VALUES (@id::uuid, @preset_id::uuid, @desired_instances::int, @invalidate_after_secs::int)
RETURNING *;

-- name: GetPrebuildMetrics :many
SELECT
    t.name as template_name,
    tvp.name as preset_name,
		COUNT(*) FILTER ( -- created
				-- TODO (sasswart): double check which job statuses should be included here
				WHERE
					pj.initiator_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0'::uuid
					AND pj.job_status = 'succeeded'::provisioner_job_status
		) as created,
		COUNT(*) FILTER ( -- failed
				-- TODO (sasswart): should we count cancelled here?
				WHERE pj.initiator_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0'::uuid
						AND pj.job_status = 'failed'::provisioner_job_status
		) as failed,
		COUNT(*) FILTER ( -- assigned
				WHERE pj.initiator_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0'::uuid
						AND NOT w.owner_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0'::uuid
		) as assigned,
		COUNT(*) FILTER ( -- exhausted
				-- TODO (sasswart): write a filter to count this
				-- we should be able to count:
				-- - workspace builds
				-- - that have a preset id
				-- - and that preset has prebuilds enabled
				-- - and the job for the prebuild was initiated by a user other than the prebuilds user
				WHERE
					wb.template_version_preset_id IS NOT NULL
					AND w.owner_id != 'c42fdf75-3097-471c-8c33-fb52454d81c0'::uuid
					AND wb.initiator_id != 'c42fdf75-3097-471c-8c33-fb52454d81c0'::uuid
		) as exhausted,
		COUNT(*) FILTER ( -- used_preset
			WHERE wb.template_version_preset_id IS NOT NULL
		) as used_preset
FROM workspace_builds wb
INNER JOIN provisioner_jobs pj ON wb.job_id = pj.id
LEFT JOIN workspaces w ON wb.workspace_id = w.id
LEFT JOIN template_version_presets tvp ON wb.template_version_preset_id = tvp.id
LEFT JOIN template_versions tv ON tv.id = wb.template_version_id
LEFT JOIN templates t ON t.id = tv.template_id
WHERE pj.initiator_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0'::uuid
GROUP BY t.name, tvp.name;
