CREATE INDEX IF NOT EXISTS workspace_agents_auth_instance_id_deleted_idx ON public.workspace_agents (auth_instance_id, deleted);
