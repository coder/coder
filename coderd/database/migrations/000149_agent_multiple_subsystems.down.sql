-- Bring back the subsystem column.
ALTER TABLE workspace_agents ADD COLUMN subsystem workspace_agent_subsystem NOT NULL DEFAULT 'none';

-- Update all existing workspace_agents to have subsystem = subsystems[0] unless
-- subsystems is empty.
UPDATE workspace_agents SET subsystem = subsystems[1] WHERE cardinality(subsystems) > 0;

-- Drop the subsystems column from workspace_agents.
ALTER TABLE workspace_agents DROP COLUMN subsystems;

-- We cannot drop the "exectrace" value from the workspace_agent_subsystem type
-- because you cannot drop values from an enum type.
UPDATE workspace_agents SET subsystem = 'none' WHERE subsystem = 'exectrace';
