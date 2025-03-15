ALTER TABLE users DROP COLUMN browser_notification_subscription;

ALTER TABLE workspace_agents DROP COLUMN task_waiting_for_user_input;
ALTER TABLE workspace_agents DROP COLUMN task_completed_at;
ALTER TABLE workspace_agents DROP COLUMN task_notifications;

DROP TABLE workspace_agent_tasks;