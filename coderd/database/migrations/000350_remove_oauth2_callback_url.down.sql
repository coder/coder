-- Reverse migration: restore callback_url column from redirect_uris

-- Add back the callback_url column
ALTER TABLE oauth2_provider_apps
ADD COLUMN callback_url text;

-- Populate callback_url from the first redirect_uri
UPDATE oauth2_provider_apps
SET callback_url = redirect_uris[1]
WHERE redirect_uris IS NOT NULL AND array_length(redirect_uris, 1) > 0;

-- Remove NOT NULL and CHECK constraints from redirect_uris (restore original state)
ALTER TABLE oauth2_provider_apps
DROP CONSTRAINT IF EXISTS oauth2_provider_apps_redirect_uris_nonempty;
ALTER TABLE oauth2_provider_apps
ALTER COLUMN redirect_uris DROP NOT NULL;

COMMENT ON COLUMN oauth2_provider_apps.callback_url IS 'Legacy callback URL field (replaced by redirect_uris array)';
