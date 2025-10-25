-- Add RDP session count to workspace_agent_stats table
ALTER TABLE workspace_agent_stats ADD COLUMN session_count_rdp bigint DEFAULT 0 NOT NULL;




