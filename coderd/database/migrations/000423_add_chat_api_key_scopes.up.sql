ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat:*';
