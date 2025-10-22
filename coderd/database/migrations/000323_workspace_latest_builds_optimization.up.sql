-- Drop the dependent views
DROP VIEW workspace_prebuilds;
-- Previously created in 000314_prebuilds.up.sql
DROP VIEW workspace_latest_builds;

-- The previous version of this view had two sequential scans on two very large
-- tables. This version optimized it by using index scans (via a lateral join)
-- AND avoiding selecting builds from deleted workspaces.
CREATE VIEW workspace_latest_builds AS
SELECT
	latest_build.id,
	latest_build.workspace_id,
	latest_build.template_version_id,
	latest_build.job_id,
	latest_build.template_version_preset_id,
	latest_build.transition,
	latest_build.created_at,
	latest_build.job_status
FROM workspaces
LEFT JOIN LATERAL (
	SELECT
		workspace_builds.id AS id,
		workspace_builds.workspace_id AS workspace_id,
		workspace_builds.template_version_id AS template_version_id,
		workspace_builds.job_id AS job_id,
		workspace_builds.template_version_preset_id AS template_version_preset_id,
		workspace_builds.transition AS transition,
		workspace_builds.created_at AS created_at,
		provisioner_jobs.job_status AS job_status
	FROM
		workspace_builds
	JOIN
		provisioner_jobs
	ON
		provisioner_jobs.id = workspace_builds.job_id
	WHERE
		workspace_builds.workspace_id = workspaces.id
	ORDER BY
		build_number DESC
	LIMIT
		1
) latest_build ON TRUE
WHERE workspaces.deleted = false
ORDER BY workspaces.id ASC;

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
