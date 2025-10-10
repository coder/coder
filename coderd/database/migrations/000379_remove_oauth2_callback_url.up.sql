-- Migrate from callback_url to redirect_uris as source of truth for OAuth2 apps
-- RFC 6749 compliance: use array of redirect URIs instead of single callback URL

-- Populate redirect_uris from callback_url where redirect_uris is empty or NULL.
-- NULLIF is used to treat empty strings in callback_url as NULL.
-- If callback_url is NULL or empty, this will result in redirect_uris
-- being an array with a single NULL element. This is preferable to an empty
-- array as it will pass a CHECK for array length > 0, enforcing that all
-- apps have at least one URI entry, even if it's null.
UPDATE oauth2_provider_apps
SET redirect_uris = ARRAY[NULLIF(callback_url, '')]
WHERE (redirect_uris IS NULL OR cardinality(redirect_uris) = 0);

-- Add NOT NULL constraint to redirect_uris
ALTER TABLE oauth2_provider_apps
ALTER COLUMN redirect_uris SET NOT NULL;

-- Add CHECK constraint to ensure redirect_uris is not empty.
-- This prevents empty arrays, which could have been created by the old logic,
-- and ensures data integrity going forward.
ALTER TABLE oauth2_provider_apps
ADD CONSTRAINT redirect_uris_not_empty CHECK (cardinality(redirect_uris) > 0);

-- Drop the callback_url column entirely
ALTER TABLE oauth2_provider_apps
DROP COLUMN callback_url;

COMMENT ON COLUMN oauth2_provider_apps.redirect_uris IS 'RFC 6749 compliant list of valid redirect URIs for the application';
