-- name: GetTemplatePrebuildState :many
WITH
	-- All prebuilds currently running
	running_prebuilds AS (SELECT p.template_id,
								 b.template_version_id,
								 COUNT(*)                    AS count,
								 STRING_AGG(p.id::text, ',') AS ids
						  FROM workspace_prebuilds p
								   INNER JOIN workspace_latest_build b ON b.workspace_id = p.id
								   INNER JOIN provisioner_jobs pj ON b.job_id = pj.id
								   INNER JOIN templates t ON p.template_id = t.id
						  WHERE (b.transition = 'start'::workspace_transition
							  -- if a deletion job fails, the workspace will still be running
							  OR pj.job_status IN ('failed'::provisioner_job_status, 'canceled'::provisioner_job_status,
												   'unknown'::provisioner_job_status))
						  GROUP BY p.template_id, b.template_version_id),
	-- All templates which have been configured for prebuilds (any version)
	templates_with_prebuilds AS (SELECT t.id                        AS template_id,
										tv.id                       AS template_version_id,
										tv.id = t.active_version_id AS using_active_version,
										tvpp.desired_instances,
										t.deleted,
										t.deprecated != ''          AS deprecated
								 FROM templates t
										  INNER JOIN template_versions tv ON tv.template_id = t.id
										  INNER JOIN template_version_presets tvp ON tvp.template_version_id = tv.id
										  INNER JOIN template_version_preset_prebuilds tvpp ON tvpp.preset_id = tvp.id
								 WHERE t.id = @template_id::uuid
								 GROUP BY t.id, tv.id, tvpp.id),
	-- Jobs relating to prebuilds current in-flight
	prebuilds_in_progress AS (SELECT wpb.template_version_id, wpb.transition, COUNT(wpb.transition) AS count
							  FROM workspace_latest_build wlb
									   INNER JOIN provisioner_jobs pj ON wlb.job_id = pj.id
									   INNER JOIN workspace_prebuild_builds wpb ON wpb.id = wlb.id
							  WHERE pj.job_status NOT IN
									('succeeded'::provisioner_job_status, 'canceled'::provisioner_job_status,
									 'failed'::provisioner_job_status)
							  GROUP BY wpb.template_version_id, wpb.transition)
SELECT t.template_id,
	   t.template_version_id,
	   t.using_active_version     AS is_active,
	   CAST(COALESCE(MAX(CASE WHEN p.template_version_id = t.template_version_id THEN p.ids END),
					 '') AS TEXT) AS running_prebuild_ids,
	   CAST(COALESCE(MAX(CASE WHEN t.using_active_version THEN p.count ELSE 0 END),
					 0) AS INT)   AS actual,     -- running prebuilds for active version
	   CAST(COALESCE(MAX(CASE WHEN t.using_active_version THEN t.desired_instances ELSE 0 END),
					 0) AS INT)   AS desired,    -- we only care about the active version's desired instances
	   CAST(COALESCE(MAX(CASE
							 WHEN p.template_version_id = t.template_version_id AND t.using_active_version = false
								 THEN p.count END),
					 0) AS INT)   AS extraneous, -- running prebuilds for inactive version
	   CAST(COALESCE(MAX(CASE WHEN pip.transition = 'start'::workspace_transition THEN pip.count ELSE 0 END),
					 0) AS INT)   AS starting,
	   CAST(COALESCE(MAX(CASE WHEN pip.transition = 'stop'::workspace_transition THEN pip.count ELSE 0 END),
					 0) AS INT)   AS stopping,   -- not strictly needed, since prebuilds should never be left if a "stopped" state, but useful to know
	   CAST(COALESCE(MAX(CASE WHEN pip.transition = 'delete'::workspace_transition THEN pip.count ELSE 0 END),
					 0) AS INT)   AS deleting,
	   t.deleted                  AS template_deleted,
	   t.deprecated               AS template_deprecated
FROM templates_with_prebuilds t
		 LEFT JOIN running_prebuilds p ON p.template_version_id = t.template_version_id
		 LEFT JOIN prebuilds_in_progress pip ON pip.template_version_id = t.template_version_id
GROUP BY t.using_active_version, t.template_id, t.template_version_id, p.count, p.ids,
		 p.template_version_id, t.deleted, t.deprecated;
