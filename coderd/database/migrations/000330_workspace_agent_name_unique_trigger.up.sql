CREATE OR REPLACE FUNCTION check_workspace_agent_name_unique()
RETURNS TRIGGER AS $$
DECLARE
    workspace_build_id uuid;
    existing_count integer;
BEGIN
    -- Get the workspace_build.id for this agent by following the relationship chain:
    -- workspace_agents -> workspace_resources -> provisioner_jobs -> workspace_builds
    SELECT wb.id INTO workspace_build_id
    FROM workspace_resources wr
    JOIN provisioner_jobs pj ON wr.job_id = pj.id
    JOIN workspace_builds wb ON pj.id = wb.job_id
    WHERE wr.id = NEW.resource_id;

    -- If we couldn't find a workspace_build_id, allow the insert (might be a template import or other edge case)
    IF workspace_build_id IS NULL THEN
        RETURN NEW;
    END IF;

    -- Check if there's already an agent with this name for this workspace build
    SELECT COUNT(*) INTO existing_count
    FROM workspace_agents wa
    JOIN workspace_resources wr ON wa.resource_id = wr.id
    JOIN provisioner_jobs pj ON wr.job_id = pj.id
    JOIN workspace_builds wb ON pj.id = wb.job_id
    WHERE wb.id = workspace_build_id
      AND wa.name = NEW.name
      AND wa.id != NEW.id;  -- Exclude the current agent (for updates)

    -- If there's already an agent with this name, raise an error
    IF existing_count > 0 THEN
        RAISE EXCEPTION 'workspace agent name "%" already exists in this workspace', NEW.name
            USING ERRCODE = 'unique_violation';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER workspace_agent_name_unique_trigger
    BEFORE INSERT OR UPDATE OF name, resource_id ON workspace_agents
    FOR EACH ROW
    EXECUTE FUNCTION check_workspace_agent_name_unique();
