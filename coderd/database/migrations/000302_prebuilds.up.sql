CREATE VIEW workspace_latest_build AS
SELECT DISTINCT ON (workspace_id) *
FROM workspace_builds
ORDER BY workspace_id, build_number DESC;

CREATE VIEW workspace_prebuilds AS
WITH
	-- All workspaces owned by the "prebuilds" user.
	all_prebuilds AS (SELECT w.*
					  FROM workspaces w
					  WHERE w.owner_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0'), -- The system user responsible for prebuilds.
	-- All workspace agents belonging to the workspaces owned by the "prebuilds" user.
	workspace_agents AS (SELECT w.id AS workspace_id, wa.id AS agent_id, wa.lifecycle_state, wa.ready_at
						 FROM workspaces w
								  INNER JOIN workspace_latest_build wlb ON wlb.workspace_id = w.id
								  INNER JOIN workspace_resources wr ON wr.job_id = wlb.job_id
								  INNER JOIN workspace_agents wa ON wa.resource_id = wr.id
						 WHERE w.owner_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0' -- The system user responsible for prebuilds.
						 GROUP BY w.id, wa.id),
	-- We can't rely on the template_version_preset_id in the workspace_builds table because this value is only set on the
	-- initial workspace creation. Subsequent stop/start transitions will not have a value for template_version_preset_id,
	-- and therefore we can't rely on (say) the latest build's chosen template_version_preset_id.
	--
	-- See https://github.com/coder/internal/issues/398
	current_presets AS (SELECT w.id AS prebuild_id, lps.template_version_preset_id
						FROM workspaces w
								 INNER JOIN (
							-- The latest workspace build which had a preset explicitly selected
							SELECT wb.*
							FROM (SELECT tv.template_id,
										 wbmax.workspace_id,
										 MAX(wbmax.build_number) as max_build_number
								  FROM workspace_builds wbmax
										   JOIN template_versions tv ON (tv.id = wbmax.template_version_id)
								  WHERE wbmax.template_version_preset_id IS NOT NULL
								  GROUP BY tv.template_id, wbmax.workspace_id) wbmax
									 JOIN workspace_builds wb ON (
								wb.workspace_id = wbmax.workspace_id
									AND wb.build_number = wbmax.max_build_number
								)) lps ON lps.workspace_id = w.id
						WHERE w.owner_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0') -- The system user responsible for prebuilds.
SELECT p.*, a.agent_id, a.lifecycle_state, a.ready_at, cp.template_version_preset_id AS current_preset_id
FROM all_prebuilds p
		 LEFT JOIN workspace_agents a ON a.workspace_id = p.id
		 INNER JOIN current_presets cp ON cp.prebuild_id = p.id;

CREATE VIEW workspace_prebuild_builds AS
SELECT *
FROM workspace_builds
WHERE initiator_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0'; -- The system user responsible for prebuilds.
