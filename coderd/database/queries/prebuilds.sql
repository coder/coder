-- name: GetTemplatePrebuildState :many
WITH latest_workspace_builds AS (SELECT wb.id,
										wb.workspace_id,
										wbmax.template_id,
										wb.template_version_id
								 FROM (SELECT tv.template_id,
											  wbmax.workspace_id,
											  MAX(wbmax.build_number) as max_build_number
									   FROM workspace_builds wbmax
												JOIN template_versions tv ON (tv.id = wbmax.template_version_id)
									   GROUP BY tv.template_id, wbmax.workspace_id) wbmax
										  JOIN workspace_builds wb ON (
									 wb.workspace_id = wbmax.workspace_id
										 AND wb.build_number = wbmax.max_build_number
									 ))
-- TODO: need to store the desired instances & autoscaling schedules in db; use desired value here
SELECT CAST(1 AS integer)           AS desired,
	   CAST(COUNT(wp.*) AS integer) AS actual,
	   CAST(0 AS integer)           AS extraneous, -- TODO: calculate this by subtracting actual from count not matching template version
	   t.deleted,
	   t.deprecated,
	   tv.id                        AS template_version_id
FROM latest_workspace_builds lwb
		 JOIN template_versions tv ON lwb.template_version_id = tv.id
		 JOIN templates t ON t.id = lwb.template_id
		 LEFT JOIN workspace_prebuilds wp ON wp.id = lwb.workspace_id
WHERE t.id = @template_id::uuid
GROUP BY t.id, t.deleted, t.deprecated, tv.id;
