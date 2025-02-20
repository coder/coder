-- name: GetRunningPrebuilds :many
SELECT p.id               AS workspace_id,
	   p.template_id,
	   b.template_version_id,
	   tvp_curr.id        AS current_preset_id,
	   tvp_desired.id     AS desired_preset_id,
	   CASE
		   WHEN p.lifecycle_state = 'ready'::workspace_agent_lifecycle_state THEN TRUE
		   ELSE FALSE END AS eligible
FROM workspace_prebuilds p
		 INNER JOIN workspace_latest_build b ON b.workspace_id = p.id
		 INNER JOIN provisioner_jobs pj ON b.job_id = pj.id
		 INNER JOIN templates t ON p.template_id = t.id
		 LEFT JOIN template_version_presets tvp_curr
				   ON tvp_curr.id = b.template_version_preset_id
		 LEFT JOIN template_version_presets tvp_desired
				   ON tvp_desired.template_version_id = t.active_version_id
WHERE (b.transition = 'start'::workspace_transition
	-- if a deletion job fails, the workspace will still be running
	OR pj.job_status IN ('failed'::provisioner_job_status, 'canceled'::provisioner_job_status,
						 'unknown'::provisioner_job_status))
  AND (tvp_curr.name = tvp_desired.name
	OR tvp_desired.id IS NULL);

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
SELECT wpb.template_version_id, wpb.transition, COUNT(wpb.transition) AS count
FROM workspace_latest_build wlb
		 INNER JOIN provisioner_jobs pj ON wlb.job_id = pj.id
		 INNER JOIN workspace_prebuild_builds wpb ON wpb.id = wlb.id
WHERE pj.job_status NOT IN
	  ('succeeded'::provisioner_job_status, 'canceled'::provisioner_job_status,
	   'failed'::provisioner_job_status)
GROUP BY wpb.template_version_id, wpb.transition;

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
