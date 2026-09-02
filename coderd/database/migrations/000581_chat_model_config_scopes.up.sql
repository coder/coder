ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_model_config:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_model_config:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_model_config:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_model_config:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_model_config:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_model_config:share';
