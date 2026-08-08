-- Add "exectrace" to workspace_agent_subsystem type.
ALTER TYPE workspace_agent_subsystem ADD VALUE 'exectrace';

-- Create column subsystems in workspace_agents table, with default value being
-- an empty array.
ALTER TABLE workspace_agents ADD COLUMN subsystems workspace_agent_subsystem[] DEFAULT '{}';

-- Add a constraint that the subsystems cannot contain the deprecated value
-- 'none'.
ALTER TABLE workspace_agents ADD CONSTRAINT subsystems_not_none CHECK (NOT ('none' = ANY (subsystems)));

-- Update all existing workspace_agents to have subsystems = [subsystem] unless
-- the subsystem is 'none'.
UPDATE workspace_agents SET subsystems = ARRAY[subsystem] WHERE subsystem != 'none';

-- Drop the subsystem column from workspace_agents.
ALTER TABLE workspace_agents DROP COLUMN subsystem;
