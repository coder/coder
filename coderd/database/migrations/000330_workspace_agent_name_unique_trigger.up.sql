CREATE OR REPLACE FUNCTION check_workspace_agent_name_unique()
RETURNS TRIGGER AS $$
DECLARE
    provisioner_job_id uuid;
    agents_with_name integer;
BEGIN
	-- Count how many agents in this resource already have
	-- the given agent ID.
	SELECT COUNT(*) INTO agents_with_name
	FROM workspace_agents
	WHERE workspace_agents.resource_id = NEW.resource_id
	  AND workspace_agents.name = NEW.name
	  AND workspace_agents.id != NEW.id;

	-- If there's already an agent with this name, raise an error
    IF agents_with_name > 0 THEN
        RAISE EXCEPTION 'workspace agent name "%" already exists in this workspace resource', NEW.name
            USING ERRCODE = 'unique_violation';
    END IF;

    -- Get the provisioner_jobs.id for this agent by following the relationship chain:
    -- workspace_agents -> workspace_resources -> provisioner_jobs
    -- SELECT pj.id INTO provisioner_job_id
    -- FROM workspace_resources wr
    -- JOIN provisioner_jobs pj ON wr.job_id = pj.id
    -- WHERE wr.id = NEW.resource_id;

    -- If we couldn't find a provisioner_job.id, allow the insert (might be a template import or other edge case)
    -- IF provisioner_job_id IS NULL THEN
    --     RETURN NEW;
    -- END IF;

    -- Check if there's already an agent with this name for this provisioner job
    -- SELECT COUNT(*) INTO existing_count
    -- FROM workspace_agents wa
    -- JOIN workspace_resources wr ON wa.resource_id = wr.id
    -- JOIN provisioner_jobs pj ON wr.job_id = pj.id
    -- WHERE pj.id = provisioner_job_id
    --   AND wa.name = NEW.name
    --   AND wa.id != NEW.id;

    -- If there's already an agent with this name, raise an error
    -- IF existing_count > 0 THEN
    --     RAISE EXCEPTION 'workspace agent name "%" already exists in this provisioner job', NEW.name
    --         USING ERRCODE = 'unique_violation';
    -- END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER workspace_agent_name_unique_trigger
    BEFORE INSERT OR UPDATE OF name, resource_id ON workspace_agents
    FOR EACH ROW
    EXECUTE FUNCTION check_workspace_agent_name_unique();
