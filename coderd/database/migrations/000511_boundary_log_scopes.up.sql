-- Add boundary_log scopes for RBAC.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'boundary_log:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'boundary_log:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'boundary_log:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'boundary_log:read';
