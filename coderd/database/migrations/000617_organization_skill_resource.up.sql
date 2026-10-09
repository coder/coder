ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'organization_skill';

ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization_skill:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization_skill:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization_skill:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization_skill:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization_skill:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'organization_skill:share';
