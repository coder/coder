-- Restore the old scope columns
ALTER TABLE api_keys ADD COLUMN scope api_key_scope NOT NULL DEFAULT 'all';
ALTER TABLE oauth2_provider_apps ADD COLUMN scope text NOT NULL DEFAULT '';

-- Migrate data back from scopes array to single scope column
UPDATE api_keys SET scope =
    CASE
        WHEN array_length(scopes, 1) IS NULL OR array_length(scopes, 1) = 0 THEN 'all'
        ELSE scopes[1]
    END;

UPDATE oauth2_provider_apps SET scope =
    CASE
        WHEN array_length(scopes, 1) IS NULL OR array_length(scopes, 1) = 0 THEN ''
        ELSE array_to_string(scopes, ' ')
    END;

-- Drop the scopes array columns
ALTER TABLE api_keys DROP COLUMN scopes;
ALTER TABLE oauth2_provider_apps DROP COLUMN scopes;

-- Note: PostgreSQL doesn't support removing enum values directly
-- This migration would require recreating the enum type entirely
-- For safety, we don't remove the new enum values
