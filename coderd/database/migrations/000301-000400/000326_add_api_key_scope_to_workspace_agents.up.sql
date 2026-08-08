-- Create the enum type for API key scope
CREATE TYPE agent_key_scope_enum AS ENUM ('all', 'no_user_data');

-- Add the api_key_scope column to the workspace_agents table
-- It defaults to 'all' to maintain existing behavior for current agents.
ALTER TABLE workspace_agents
ADD COLUMN api_key_scope agent_key_scope_enum NOT NULL DEFAULT 'all';

-- Add a comment explaining the purpose of the column
COMMENT ON COLUMN workspace_agents.api_key_scope IS 'Defines the scope of the API key associated with the agent. ''all'' allows access to everything, ''no_user_data'' restricts it to exclude user data.';
