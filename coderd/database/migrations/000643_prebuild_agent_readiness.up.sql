CREATE FUNCTION prebuild_agents_ready(build_job_id uuid, connected_after timestamptz)
RETURNS boolean
LANGUAGE sql STABLE
AS $$
    SELECT
        EXISTS (
            SELECT 1 FROM workspace_resources wr
            JOIN workspace_agents wa ON wa.resource_id = wr.id
            WHERE wr.job_id = build_job_id AND NOT wa.deleted AND wa.parent_id IS NULL
        )
        AND NOT EXISTS (
            SELECT 1 FROM workspace_resources wr
            JOIN workspace_agents wa ON wa.resource_id = wr.id
            WHERE wr.job_id = build_job_id AND NOT wa.deleted
                AND (wa.lifecycle_state != 'ready'
                    OR wa.first_connected_at IS NULL
                    OR wa.last_connected_at IS NULL
                    OR wa.last_connected_at < connected_after
                    OR wa.disconnected_at >= wa.last_connected_at)
        )
        AND NOT EXISTS (
            SELECT 1 FROM workspace_resources wr
            JOIN workspace_agents wa ON wa.resource_id = wr.id
            JOIN workspace_agent_devcontainers dc ON dc.workspace_agent_id = wa.id
            LEFT JOIN workspace_agents child ON child.id = dc.subagent_id
            WHERE wr.job_id = build_job_id AND NOT wa.deleted
                AND (child.id IS NULL OR child.deleted OR child.resource_id != wr.id)
        );
$$;
