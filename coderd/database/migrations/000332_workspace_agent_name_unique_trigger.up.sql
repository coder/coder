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
		AND workspace_agents.id != NEW.id;

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
