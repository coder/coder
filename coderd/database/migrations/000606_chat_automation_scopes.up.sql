ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_automation:delete';

ALTER TYPE resource_type
	ADD VALUE IF NOT EXISTS 'chat_automation';
