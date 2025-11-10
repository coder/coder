-- Tasks RBAC.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'task:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'task:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'task:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'task:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'task:*';
