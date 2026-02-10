ALTER TABLE workspace_agents
    ADD COLUMN restart_count integer NOT NULL DEFAULT 0,
    ADD COLUMN last_restarted_at timestamp with time zone;
