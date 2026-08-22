ALTER TABLE oauth2_provider_app_codes
    DROP COLUMN state_hash,
    DROP COLUMN redirect_uri;
