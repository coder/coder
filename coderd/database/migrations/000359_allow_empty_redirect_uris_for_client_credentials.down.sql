-- Restore the original constraint that requires at least one redirect URI
-- This will fail if there are existing client credentials applications with empty redirect URIs
ALTER TABLE oauth2_provider_apps
    DROP CONSTRAINT IF EXISTS redirect_uris_not_empty_unless_client_credentials;

ALTER TABLE oauth2_provider_apps
    ADD CONSTRAINT redirect_uris_not_empty CHECK (cardinality(redirect_uris) > 0);

COMMENT ON COLUMN oauth2_provider_apps.redirect_uris IS 'RFC 6749 compliant list of valid redirect URIs for the application';

-- Remove the denormalized app_owner_user_id column
DROP INDEX IF EXISTS idx_oauth2_provider_app_secrets_app_owner_user_id;

ALTER TABLE oauth2_provider_app_secrets
    DROP COLUMN IF EXISTS app_owner_user_id;

