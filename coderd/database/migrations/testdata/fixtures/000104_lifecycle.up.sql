-- Set lifecycle_state to enum value not available in previous migration.
UPDATE workspace_agents SET lifecycle_state = 'off' WHERE id = '7a1ce5f8-8d00-431c-ad1b-97a846512804';
