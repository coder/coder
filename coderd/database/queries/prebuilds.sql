-- name: GetTemplatePrebuildState :many
WITH
	-- All prebuilds currently running
	running_prebuilds AS (SELECT p.*, b.template_version_id
						  FROM workspace_prebuilds p
								   INNER JOIN workspace_latest_build b ON b.workspace_id = p.id
						  WHERE b.transition = 'start'::workspace_transition),
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
	prebuilds_in_progress AS (SELECT wpb.template_version_id, pj.id AS job_id, pj.type, pj.job_status, wpb.transition
							  FROM workspace_prebuild_builds wpb
									   INNER JOIN workspace_latest_build wlb ON wpb.workspace_id = wlb.workspace_id
									   INNER JOIN provisioner_jobs pj ON wlb.job_id = pj.id
							  WHERE pj.job_status NOT IN
									('succeeded'::provisioner_job_status, 'canceled'::provisioner_job_status,
									 'failed'::provisioner_job_status))
SELECT t.template_id,
	   CAST(COUNT(p.id) AS INT)                                                                      AS actual,     -- running prebuilds for active version
	   CAST(MAX(CASE WHEN t.using_active_version THEN t.desired_instances ELSE 0 END) AS int)        AS desired,    -- we only care about the active version's desired instances
	   CAST(SUM(CASE WHEN t.using_active_version THEN 0 ELSE 1 END) AS INT)                          AS extraneous, -- running prebuilds for inactive version
	   CAST(SUM(CASE
					WHEN pip.transition = 'start'::workspace_transition THEN 1
					ELSE 0 END) AS INT)                                                              AS starting,
	   CAST(SUM(CASE
					WHEN pip.transition = 'delete'::workspace_transition THEN 1
					ELSE 0 END) AS INT)                                                              AS deleting,
	   t.deleted AS template_deleted,
	   t.deprecated AS template_deprecated
FROM templates_with_prebuilds t
		 LEFT JOIN running_prebuilds p ON p.template_version_id = t.template_version_id
		 LEFT JOIN prebuilds_in_progress pip ON pip.template_version_id = t.template_version_id
GROUP BY t.template_id, p.id, t.deleted, t.deprecated, pip.template_version_id;
