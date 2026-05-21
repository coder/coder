-- Add high-level composite coder:* API key scopes
-- These values are persisted so that tokens can store coder:* names directly.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'coder:workspaces.create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'coder:workspaces.operate';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'coder:workspaces.delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'coder:workspaces.access';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'coder:templates.build';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'coder:templates.author';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'coder:apikeys.manage_self';
