ALTER TABLE workspace_agents
    DROP COLUMN IF EXISTS chat_runner_ready_at,
    DROP COLUMN IF EXISTS chat_runner_ready;
