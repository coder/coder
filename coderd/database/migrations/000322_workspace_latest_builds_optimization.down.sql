DROP VIEW workspace_prebuilds;
DROP VIEW workspace_latest_builds;

-- Revert to previous version from 000314_prebuilds.up.sql
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

-- Recreate the dependent views
CREATE VIEW workspace_prebuilds AS
 WITH all_prebuilds AS (
         SELECT w.id,
            w.name,
            w.template_id,
            w.created_at
           FROM workspaces w
          WHERE (w.owner_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0'::uuid)
        ), workspaces_with_latest_presets AS (
         SELECT DISTINCT ON (workspace_builds.workspace_id) workspace_builds.workspace_id,
            workspace_builds.template_version_preset_id
           FROM workspace_builds
          WHERE (workspace_builds.template_version_preset_id IS NOT NULL)
          ORDER BY workspace_builds.workspace_id, workspace_builds.build_number DESC
        ), workspaces_with_agents_status AS (
         SELECT w.id AS workspace_id,
            bool_and((wa.lifecycle_state = 'ready'::workspace_agent_lifecycle_state)) AS ready
           FROM (((workspaces w
             JOIN workspace_latest_builds wlb ON ((wlb.workspace_id = w.id)))
             JOIN workspace_resources wr ON ((wr.job_id = wlb.job_id)))
             JOIN workspace_agents wa ON ((wa.resource_id = wr.id)))
          WHERE (w.owner_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0'::uuid)
          GROUP BY w.id
        ), current_presets AS (
         SELECT w.id AS prebuild_id,
            wlp.template_version_preset_id
           FROM (workspaces w
             JOIN workspaces_with_latest_presets wlp ON ((wlp.workspace_id = w.id)))
          WHERE (w.owner_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0'::uuid)
        )
 SELECT p.id,
    p.name,
    p.template_id,
    p.created_at,
    COALESCE(a.ready, false) AS ready,
    cp.template_version_preset_id AS current_preset_id
   FROM ((all_prebuilds p
     LEFT JOIN workspaces_with_agents_status a ON ((a.workspace_id = p.id)))
     JOIN current_presets cp ON ((cp.prebuild_id = p.id)));
