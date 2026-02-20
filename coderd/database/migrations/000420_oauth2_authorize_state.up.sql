ALTER TABLE oauth2_provider_app_codes
    ADD COLUMN state_hash text;

COMMENT ON COLUMN oauth2_provider_app_codes.state_hash IS
    'SHA-256 hash of the OAuth2 state parameter, stored to prevent state reflection attacks.';
