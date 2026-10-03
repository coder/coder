ALTER TABLE oauth2_provider_app_codes
    ADD COLUMN state_hash text,
    ADD COLUMN redirect_uri text;

COMMENT ON COLUMN oauth2_provider_app_codes.state_hash IS
    'SHA-256 hash of the OAuth2 state parameter, stored to prevent state reflection attacks.';

COMMENT ON COLUMN oauth2_provider_app_codes.redirect_uri IS
    'The redirect_uri provided during authorization, to be verified during token exchange (RFC 6749 §4.1.3).';
