ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_seat:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_seat:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'ai_seat:read';
