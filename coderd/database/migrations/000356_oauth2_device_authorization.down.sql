-- Remove OAuth2 Device Authorization Grant support (RFC 8628)

-- Remove constraints added for data integrity
ALTER TABLE ONLY oauth2_provider_apps
    DROP CONSTRAINT IF EXISTS redirect_uris_non_empty;

DROP TABLE IF EXISTS oauth2_provider_device_codes CASCADE;
DROP TYPE IF EXISTS oauth2_device_status;
