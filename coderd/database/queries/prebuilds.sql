-- name: GetTemplatePresetsWithPrebuilds :many
-- GetTemplatePresetsWithPrebuilds retrieves template versions with configured presets.
-- It also returns the number of desired instances for each preset.
-- If template_id is specified, only template versions associated with that template will be returned.
SELECT t.id                        AS template_id,
	   t.name                      AS template_name,
	   tv.id                       AS template_version_id,
	   tv.name                     AS template_version_name,
	   tv.id = t.active_version_id AS using_active_version,
       tvp.id,
	   tvp.name,
	   tvp.desired_instances       AS desired_instances,
	   t.deleted,
	   t.deprecated != ''          AS deprecated
FROM templates t
		 INNER JOIN template_versions tv ON tv.template_id = t.id
		 INNER JOIN template_version_presets tvp ON tvp.template_version_id = tv.id
WHERE tvp.desired_instances IS NOT NULL -- Consider only presets that have a prebuild configuration.
  AND (t.id = sqlc.narg('template_id')::uuid OR sqlc.narg('template_id') IS NULL);

-- name: GetRunningPrebuilds :many
SELECT p.id                AS workspace_id,
	   p.name              AS workspace_name,
	   p.template_id,
	   b.template_version_id,
       p.current_preset_id AS current_preset_id,
	   CASE
		   WHEN p.lifecycle_state = 'ready'::workspace_agent_lifecycle_state THEN TRUE
		   ELSE FALSE END AS ready,
	   p.created_at
FROM workspace_prebuilds p
		 INNER JOIN workspace_latest_builds b ON b.workspace_id = p.id
		 INNER JOIN provisioner_jobs pj ON b.job_id = pj.id -- See https://github.com/coder/internal/issues/398.
WHERE (b.transition = 'start'::workspace_transition
	AND pj.job_status = 'succeeded'::provisioner_job_status);

-- name: GetPrebuildsInProgress :many
SELECT t.id AS template_id, wpb.template_version_id, wpb.transition, COUNT(wpb.transition)::int AS count
FROM workspace_latest_builds wlb
		 INNER JOIN provisioner_jobs pj ON wlb.job_id = pj.id
		 INNER JOIN workspace_prebuild_builds wpb ON wpb.id = wlb.id
		 INNER JOIN templates t ON t.active_version_id = wlb.template_version_id
WHERE pj.job_status IN ('pending'::provisioner_job_status, 'running'::provisioner_job_status)
GROUP BY t.id, wpb.template_version_id, wpb.transition;

-- name: GetPresetsBackoff :many
-- GetPresetsBackoff groups workspace builds by template version ID.
-- For each group, the query checks the last N jobs, where N equals the number of desired instances for the corresponding preset.
-- If at least one of the last N jobs has failed, we should backoff on the corresponding template version ID.
-- Query returns a list of template version IDs for which we should backoff.
-- Only active template versions with configured presets are considered.
--
-- NOTE:
-- We back off on the template version ID if at least one of the N latest workspace builds has failed.
-- However, we also return the number of failed workspace builds that occurred during the lookback period.
--
-- In other words:
-- - To **decide whether to back off**, we look at the N most recent builds (regardless of when they happened).
-- - To **calculate the number of failed builds**, we consider all builds within the defined lookback period.
--
-- The number of failed builds is used downstream to determine the backoff duration.
WITH filtered_builds AS (
	-- Only select builds which are for prebuild creations
	SELECT wlb.*, tvp.id AS preset_id, pj.job_status, tvp.desired_instances
	FROM template_version_presets tvp
			 JOIN workspace_latest_builds wlb ON wlb.template_version_preset_id = tvp.id
			 JOIN provisioner_jobs pj ON wlb.job_id = pj.id
			 JOIN template_versions tv ON wlb.template_version_id = tv.id
			 JOIN templates t ON tv.template_id = t.id AND t.active_version_id = tv.id
	WHERE tvp.desired_instances IS NOT NULL -- Consider only presets that have a prebuild configuration.
      AND wlb.transition = 'start'::workspace_transition
),
time_sorted_builds AS (
    -- Group builds by template version, then sort each group by created_at.
	SELECT fb.*,
	    ROW_NUMBER() OVER (PARTITION BY fb.template_version_preset_id ORDER BY fb.created_at DESC) as rn
	FROM filtered_builds fb
),
failed_count AS (
    -- Count failed builds per template version/preset in the given period
	SELECT preset_id, COUNT(*) AS num_failed
	FROM filtered_builds
	WHERE job_status = 'failed'::provisioner_job_status
		AND created_at >= @lookback::timestamptz
	GROUP BY preset_id
)
SELECT tsb.template_version_id,
	   tsb.preset_id,
	   COALESCE(fc.num_failed, 0)::int  AS num_failed,
	   MAX(tsb.created_at::timestamptz) AS last_build_at
FROM time_sorted_builds tsb
		 LEFT JOIN failed_count fc ON fc.preset_id = tsb.preset_id
WHERE tsb.rn <= tsb.desired_instances -- Fetch the last N builds, where N is the number of desired instances; if any fail, we backoff
  AND tsb.job_status = 'failed'::provisioner_job_status
GROUP BY tsb.template_version_id, tsb.preset_id, fc.num_failed;

-- name: ClaimPrebuild :one
UPDATE workspaces w
SET owner_id   = @new_user_id::uuid,
	name       = @new_name::text,
	updated_at = NOW()
WHERE w.id IN (SELECT p.id
			   FROM workspace_prebuilds p
						INNER JOIN workspace_latest_builds b ON b.workspace_id = p.id
						INNER JOIN provisioner_jobs pj ON b.job_id = pj.id
						INNER JOIN templates t ON p.template_id = t.id
			   WHERE (b.transition = 'start'::workspace_transition
				   AND pj.job_status IN ('succeeded'::provisioner_job_status))
				 AND b.template_version_id = t.active_version_id
				 AND b.template_version_preset_id = @preset_id::uuid
				 AND p.lifecycle_state = 'ready'::workspace_agent_lifecycle_state
			   ORDER BY random()
			   LIMIT 1 FOR UPDATE OF p SKIP LOCKED) -- Ensure that a concurrent request will not select the same prebuild.
RETURNING w.id, w.name;

-- name: GetPrebuildMetrics :many
SELECT
    t.name as template_name,
    tvp.name as preset_name,
    COUNT(*) as created_count,
    COUNT(*) FILTER (WHERE pj.job_status = 'failed'::provisioner_job_status) as failed_count,
    COUNT(*) FILTER (
			 WHERE w.owner_id != 'c42fdf75-3097-471c-8c33-fb52454d81c0'::uuid -- The system user responsible for prebuilds.
		) as claimed_count
FROM workspaces w
INNER JOIN workspace_prebuild_builds wpb ON wpb.workspace_id = w.id
INNER JOIN templates t ON t.id = w.template_id
INNER JOIN template_version_presets tvp ON tvp.id = wpb.template_version_preset_id
INNER JOIN provisioner_jobs pj ON pj.id = wpb.job_id
WHERE wpb.build_number = 1
GROUP BY t.name, tvp.name
ORDER BY t.name, tvp.name;
