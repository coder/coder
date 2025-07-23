-- Extend existing api_key_scope enum with new granular scopes
ALTER TYPE api_key_scope ADD VALUE 'user:read';
ALTER TYPE api_key_scope ADD VALUE 'user:write';
ALTER TYPE api_key_scope ADD VALUE 'workspace:read';
ALTER TYPE api_key_scope ADD VALUE 'workspace:write';
ALTER TYPE api_key_scope ADD VALUE 'workspace:ssh';
ALTER TYPE api_key_scope ADD VALUE 'workspace:apps';
ALTER TYPE api_key_scope ADD VALUE 'template:read';
ALTER TYPE api_key_scope ADD VALUE 'template:write';
ALTER TYPE api_key_scope ADD VALUE 'organization:read';
ALTER TYPE api_key_scope ADD VALUE 'organization:write';
ALTER TYPE api_key_scope ADD VALUE 'audit:read';
ALTER TYPE api_key_scope ADD VALUE 'system:read';
ALTER TYPE api_key_scope ADD VALUE 'system:write';

-- Add new scopes column as enum array to api_keys table
ALTER TABLE api_keys ADD COLUMN scopes api_key_scope[];

-- Migrate existing data: convert single scope to array
UPDATE api_keys SET scopes = ARRAY[scope] WHERE scopes IS NULL;

-- Make scopes column non-null with default
ALTER TABLE api_keys ALTER COLUMN scopes SET NOT NULL;
ALTER TABLE api_keys ALTER COLUMN scopes SET DEFAULT '{"all"}';

-- Add new scopes column as enum array to oauth2_provider_apps table
ALTER TABLE oauth2_provider_apps ADD COLUMN scopes api_key_scope[];

-- Migrate existing data: split space-delimited scopes and convert to enum array
UPDATE oauth2_provider_apps SET scopes =
    CASE
        WHEN scope = '' THEN '{}'::api_key_scope[]
        ELSE string_to_array(scope, ' ')::api_key_scope[]
    END
WHERE scopes IS NULL;

-- Make scopes column non-null
ALTER TABLE oauth2_provider_apps ALTER COLUMN scopes SET NOT NULL;

-- Remove the old scope columns as they are now replaced by the scopes array
ALTER TABLE api_keys DROP COLUMN scope;
ALTER TABLE oauth2_provider_apps DROP COLUMN scope;
