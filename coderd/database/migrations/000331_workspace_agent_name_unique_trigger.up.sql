CREATE OR REPLACE FUNCTION check_workspace_agent_name_unique()
RETURNS TRIGGER AS $$
DECLARE
    provisioner_job_id uuid;
    agents_with_name integer;
BEGIN
	IF NEW.parent_id IS NULL THEN
		-- Count how many agents in this resource already have
		-- the given agent name.
		SELECT COUNT(*) INTO agents_with_name
		FROM workspace_agents
		WHERE workspace_agents.resource_id = NEW.resource_id
		  AND workspace_agents.name = NEW.name
		  AND workspace_agents.id != NEW.id;
	ELSE
		SELECT provisioner_jobs.id INTO provisioner_job_id
		FROM workspace_resources
		JOIN provisioner_jobs ON provisioner_jobs.id = workspace_resources.job_id
		WHERE workspace_resources.id = NEW.resource_id;

		-- Count how many agents in this provisioner job already have
		-- the given agent name.
		SELECT COUNT(*) INTO agents_with_name
		FROM workspace_agents
		JOIN workspace_resources ON workspace_resources.id = workspace_agents.resource_id
		JOIN provisioner_jobs ON provisioner_jobs.id = workspace_resources.job_id
		WHERE provisioner_jobs.id = provisioner_job_id
		  AND workspace_agents.name = NEW.name
		  AND workspace_agents.id != NEW.id;
	END IF;

	-- If there's already an agent with this name, raise an error
    IF agents_with_name > 0 THEN
        RAISE EXCEPTION 'workspace agent name "%" already exists in this workspace resource', NEW.name
            USING ERRCODE = 'unique_violation';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER workspace_agent_name_unique_trigger
    BEFORE INSERT OR UPDATE OF name, resource_id ON workspace_agents
    FOR EACH ROW
    EXECUTE FUNCTION check_workspace_agent_name_unique();
