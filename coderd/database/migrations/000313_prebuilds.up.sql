-- workspace_latest_builds contains latest build for every workspace
CREATE VIEW workspace_latest_builds AS
SELECT DISTINCT ON (workspace_id)
	wb.id,
	wb.workspace_id,
	wb.template_version_id,
	wb.job_id,
	wb.template_version_preset_id,
	wb.transition,
	wb.created_at,
	pj.job_status
FROM workspace_builds wb
	INNER JOIN provisioner_jobs pj ON wb.job_id = pj.id
ORDER BY wb.workspace_id, wb.build_number DESC;

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
    workspaces_with_latest_presets AS (
        SELECT DISTINCT ON (workspace_id) workspace_id, template_version_preset_id
        FROM workspace_builds
        WHERE template_version_preset_id IS NOT NULL
        ORDER BY workspace_id, build_number DESC
    ),
	-- workspaces_with_agents_status contains workspaces owned by the "prebuilds" user,
	-- along with the readiness status of their agents.
	-- A workspace is marked as 'ready' only if ALL of its agents are ready.
	workspaces_with_agents_status AS (
		SELECT w.id AS workspace_id,
		       BOOL_AND(wa.lifecycle_state = 'ready'::workspace_agent_lifecycle_state) AS ready
		FROM workspaces w
			INNER JOIN workspace_latest_builds wlb ON wlb.workspace_id = w.id
			INNER JOIN workspace_resources wr ON wr.job_id = wlb.job_id
			INNER JOIN workspace_agents wa ON wa.resource_id = wr.id
		WHERE w.owner_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0' -- The system user responsible for prebuilds.
		GROUP BY w.id
	),
    current_presets AS (SELECT w.id AS prebuild_id, wlp.template_version_preset_id
                        FROM workspaces w
                                 INNER JOIN workspaces_with_latest_presets wlp ON wlp.workspace_id = w.id
                        WHERE w.owner_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0') -- The system user responsible for prebuilds.
SELECT p.id, p.name, p.template_id, p.created_at, COALESCE(a.ready, false) AS ready, cp.template_version_preset_id AS current_preset_id
FROM all_prebuilds p
         LEFT JOIN workspaces_with_agents_status a ON a.workspace_id = p.id
         INNER JOIN current_presets cp ON cp.prebuild_id = p.id;

CREATE VIEW workspace_prebuild_builds AS
SELECT id, workspace_id, template_version_id, transition, job_id, template_version_preset_id, build_number
FROM workspace_builds
WHERE initiator_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0'; -- The system user responsible for prebuilds.
