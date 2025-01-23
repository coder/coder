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
								 GROUP BY t.id, tv.id, tvpp.id)
SELECT t.template_id,
	   COUNT(p.id)                                                               AS actual,     -- running prebuilds for active version
	   MAX(CASE WHEN t.using_active_version THEN t.desired_instances ELSE 0 END) AS desired,    -- we only care about the active version's desired instances
	   SUM(CASE WHEN t.using_active_version THEN 0 ELSE 1 END)                   AS extraneous, -- running prebuilds for inactive version
	   t.deleted,
	   t.deprecated
FROM templates_with_prebuilds t
		 LEFT JOIN running_prebuilds p ON p.template_version_id = t.template_version_id
GROUP BY t.template_id, p.id, t.deleted, t.deprecated;
