CREATE OR REPLACE FUNCTION check_workspace_agent_name_unique()
RETURNS TRIGGER AS $$
DECLARE
	provisioner_job_id uuid;
	workspace_build_count int;
	agents_with_name int;
BEGIN
	-- Find the provisioner job the workspace agent is being inserted into.
	SELECT provisioner_jobs.id INTO provisioner_job_id
	FROM workspace_resources
	JOIN provisioner_jobs ON provisioner_jobs.id = workspace_resources.job_id
	WHERE workspace_resources.id = NEW.resource_id;

	-- Get whether the provisioner job has an associated workspace build
	SELECT COUNT(*) INTO workspace_build_count
	FROM workspace_builds
	WHERE workspace_builds.job_id = provisioner_job_id;

	-- If the provisioner job doesn't have a workspace build, we'll just
	-- allow this.
	IF workspace_build_count = 0 THEN
		RETURN NEW;
	END IF;

	-- Count how many agents in this provisioner job already have
	-- the given agent name.
	SELECT COUNT(*) INTO agents_with_name
	FROM workspace_agents
	JOIN workspace_resources ON workspace_resources.id = workspace_agents.resource_id
	JOIN provisioner_jobs ON provisioner_jobs.id = workspace_resources.job_id
	WHERE provisioner_jobs.id = provisioner_job_id
	  AND workspace_agents.name = NEW.name
	  AND workspace_agents.id != NEW.id;

	-- If there's already an agent with this name, raise an error
	IF agents_with_name > 0 THEN
		RAISE EXCEPTION 'workspace agent name "%" already exists in this provisioner job', NEW.name
			USING ERRCODE = 'unique_violation';
	END IF;

	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER workspace_agent_name_unique_trigger
	BEFORE INSERT OR UPDATE OF name, resource_id ON workspace_agents
	FOR EACH ROW
	EXECUTE FUNCTION check_workspace_agent_name_unique();
