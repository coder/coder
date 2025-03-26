-- workspace_latest_builds contains latest build for every workspace
CREATE VIEW workspace_latest_builds AS
SELECT DISTINCT ON (workspace_id) *
FROM workspace_builds
ORDER BY workspace_id, build_number DESC;

-- workspace_prebuilds contains all prebuilt workspaces with corresponding agent information
-- (including lifecycle_state which indicates is agent ready or not) and corresponding preset_id for prebuild
CREATE VIEW workspace_prebuilds AS
WITH
    -- All workspaces owned by the "prebuilds" user.
    all_prebuilds AS (
		SELECT w.id, w.name, w.template_id, w.created_at
		FROM workspaces w
		WHERE w.owner_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0' -- The system user responsible for prebuilds.
	),
    -- We can't rely on the template_version_preset_id in the workspace_builds table because this value is only set on the
    -- initial workspace creation. Subsequent stop/start transitions will not have a value for template_version_preset_id,
    -- and therefore we can't rely on (say) the latest build's chosen template_version_preset_id.
    --
    -- See https://github.com/coder/internal/issues/398
    latest_prebuild_builds AS (
        SELECT DISTINCT ON (workspace_id) workspace_id, template_version_preset_id
        FROM workspace_builds
        WHERE template_version_preset_id IS NOT NULL
        ORDER BY workspace_id, build_number DESC
    ),
    -- All workspace agents belonging to the workspaces owned by the "prebuilds" user.
    workspace_agents AS (
		SELECT w.id AS workspace_id, wa.id AS agent_id, wa.lifecycle_state, wa.ready_at
		FROM workspaces w
			INNER JOIN workspace_latest_builds wlb ON wlb.workspace_id = w.id
			INNER JOIN workspace_resources wr ON wr.job_id = wlb.job_id
			INNER JOIN workspace_agents wa ON wa.resource_id = wr.id
		WHERE w.owner_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0' -- The system user responsible for prebuilds.
		GROUP BY w.id, wa.id
	),
    current_presets AS (SELECT w.id AS prebuild_id, lpb.template_version_preset_id
                        FROM workspaces w
                                 INNER JOIN latest_prebuild_builds lpb ON lpb.workspace_id = w.id
                        WHERE w.owner_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0') -- The system user responsible for prebuilds.
SELECT p.id, p.name, p.template_id, p.created_at, a.agent_id, a.lifecycle_state, a.ready_at, cp.template_version_preset_id AS current_preset_id
FROM all_prebuilds p
         LEFT JOIN workspace_agents a ON a.workspace_id = p.id
         INNER JOIN current_presets cp ON cp.prebuild_id = p.id;

CREATE VIEW workspace_prebuild_builds AS
SELECT *
FROM workspace_builds
WHERE initiator_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0'; -- The system user responsible for prebuilds.
