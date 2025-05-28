CREATE OR REPLACE FUNCTION check_workspace_agent_name_unique()
RETURNS TRIGGER AS $$
DECLARE
    provisioner_job_id_var uuid;
    workspace_id_var uuid;
    agents_with_name_var int;
BEGIN
	-- Find the provisioner job and workspace the agent is
	-- being inserted into.
	SELECT INTO provisioner_job_id_var, workspace_id_var
		provisioner_jobs.id, workspaces.id
	FROM workspace_resources
	JOIN provisioner_jobs ON provisioner_jobs.id = workspace_resources.job_id
	JOIN workspace_builds ON workspace_builds.job_id = provisioner_jobs.id
	JOIN workspaces ON workspaces.id = workspace_builds.workspace_id
	WHERE workspace_resources.id = NEW.resource_id;

	-- If there is no workspace or provisioner job attached to the agent,
	-- we will allow the insert to happen as there is no need to guarantee
	-- uniqueness.
	IF workspace_id_var IS NULL OR provisioner_job_id_var IS NULL THEN
		RETURN NEW;
	END IF;

	-- Count how many agents in this provisioner job already have
	-- the given agent name.
	SELECT COUNT(*) INTO agents_with_name_var
	FROM workspace_agents
	JOIN workspace_resources ON workspace_resources.id = workspace_agents.resource_id
	JOIN provisioner_jobs ON provisioner_jobs.id = workspace_resources.job_id
	JOIN workspace_builds ON workspace_builds.job_id = provisioner_jobs.id
	WHERE provisioner_jobs.id = provisioner_job_id_var
	  AND workspace_builds.workspace_id = workspace_id_var
	  AND workspace_agents.name = NEW.name
	  AND workspace_agents.id != NEW.id;

	-- If there's already an agent with this name, raise an error
    IF agents_with_name_var > 0 THEN
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
