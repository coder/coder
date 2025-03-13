CREATE TABLE workspace_agent_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES workspace_agents(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reporter TEXT NOT NULL,
    summary TEXT NOT NULL,
    link_to TEXT NOT NULL,
    icon TEXT NOT NULL
);

ALTER TABLE workspace_agents ADD COLUMN task_waiting_for_user_input BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE workspace_agents ADD COLUMN task_completed_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE workspace_agents ADD COLUMN task_notifications BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE users ADD COLUMN browser_notification_subscription jsonb;
