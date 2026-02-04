ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace:update_agent';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'workspace_dormant:update_agent';
