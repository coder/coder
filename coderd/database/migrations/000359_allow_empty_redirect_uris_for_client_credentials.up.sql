-- Allow empty redirect URIs for client credentials applications
-- Client credentials flow doesn't require redirect URIs (RFC 6749 Section 4.4)
-- This replaces the simple constraint with a more sophisticated one
ALTER TABLE oauth2_provider_apps
    DROP CONSTRAINT IF EXISTS redirect_uris_not_empty;

-- Add a more sophisticated constraint that allows empty redirect URIs only for client credentials applications
-- For client credentials applications (grant_types = ['client_credentials']), redirect URIs can be empty
-- For all other grant types, at least one redirect URI is required
ALTER TABLE oauth2_provider_apps
    ADD CONSTRAINT redirect_uris_not_empty_unless_client_credentials CHECK ((grant_types = ARRAY['client_credentials'::text] AND cardinality(redirect_uris) >= 0) OR (grant_types != ARRAY['client_credentials'::text] AND cardinality(redirect_uris) > 0));

COMMENT ON COLUMN oauth2_provider_apps.redirect_uris IS 'RFC 6749 compliant list of valid redirect URIs for the application. May be empty for client credentials applications.';

-- Add app_owner_user_id column to oauth2_provider_app_secrets for denormalization
-- This avoids N+1 queries when authorizing secret operations
ALTER TABLE oauth2_provider_app_secrets
    ADD COLUMN app_owner_user_id UUID NULL;

-- Populate the new column with existing data from parent apps
UPDATE oauth2_provider_app_secrets
SET app_owner_user_id = oauth2_provider_apps.user_id
FROM oauth2_provider_apps
WHERE oauth2_provider_app_secrets.app_id = oauth2_provider_apps.id;

-- Add index for efficient authorization queries
CREATE INDEX IF NOT EXISTS idx_oauth2_provider_app_secrets_app_owner_user_id
    ON oauth2_provider_app_secrets(app_owner_user_id);

COMMENT ON COLUMN oauth2_provider_app_secrets.app_owner_user_id IS 'Denormalized owner user ID from parent app for efficient authorization without N+1 queries';

