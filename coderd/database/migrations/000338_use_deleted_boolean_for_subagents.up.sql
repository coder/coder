ALTER TABLE workspace_agents
  ADD COLUMN deleted BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN workspace_agents.deleted IS 'Indicates whether or not the agent has been deleted. This is currently only applicable to sub agents.';

-- Recreate the trigger with deleted check.
DROP TRIGGER IF EXISTS workspace_agent_name_unique_trigger ON workspace_agents;
DROP FUNCTION IF EXISTS check_workspace_agent_name_unique();

CREATE OR REPLACE FUNCTION check_workspace_agent_name_unique()
RETURNS TRIGGER AS $$
DECLARE
	workspace_build_id uuid;
	agents_with_name int;
BEGIN
	-- Find the workspace build the workspace agent is being inserted into.
	SELECT workspace_builds.id INTO workspace_build_id
	FROM workspace_resources
	JOIN workspace_builds ON workspace_builds.job_id = workspace_resources.job_id
	WHERE workspace_resources.id = NEW.resource_id;

	-- If the agent doesn't have a workspace build, we'll allow the insert.
	IF workspace_build_id IS NULL THEN
		RETURN NEW;
	END IF;

	-- Count how many agents in this workspace build already have the given agent name.
	SELECT COUNT(*) INTO agents_with_name
	FROM workspace_agents
	JOIN workspace_resources ON workspace_resources.id = workspace_agents.resource_id
	JOIN workspace_builds ON workspace_builds.job_id = workspace_resources.job_id
	WHERE workspace_builds.id = workspace_build_id
		AND workspace_agents.name = NEW.name
		AND workspace_agents.id != NEW.id
		AND workspace_agents.deleted = FALSE;  -- Ensure we only count non-deleted agents.

	-- If there's already an agent with this name, raise an error
	IF agents_with_name > 0 THEN
		RAISE EXCEPTION 'workspace agent name "%" already exists in this workspace build', NEW.name
			USING ERRCODE = 'unique_violation';
	END IF;

	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER workspace_agent_name_unique_trigger
	BEFORE INSERT OR UPDATE OF name, resource_id ON workspace_agents
	FOR EACH ROW
	EXECUTE FUNCTION check_workspace_agent_name_unique();

COMMENT ON TRIGGER workspace_agent_name_unique_trigger ON workspace_agents IS
'Use a trigger instead of a unique constraint because existing data may violate
the uniqueness requirement. A trigger allows us to enforce uniqueness going
forward without requiring a migration to clean up historical data.';

-- Handle agent deletion in prebuilds, previously modified in 000323_workspace_latest_builds_optimization.up.sql.
DROP VIEW workspace_prebuilds;

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
			 -- ADD: deleted check for sub agents.
             JOIN workspace_agents wa ON ((wa.resource_id = wr.id AND wa.deleted = FALSE)))
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
