ALTER TABLE workspace_agents
    ADD COLUMN chat_runner_ready BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN chat_runner_ready_at TIMESTAMPTZ;
