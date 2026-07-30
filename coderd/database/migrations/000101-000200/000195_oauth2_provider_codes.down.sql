DROP TRIGGER IF EXISTS trigger_delete_oauth2_provider_app_token ON oauth2_provider_app_tokens;
DROP FUNCTION IF EXISTS delete_deleted_oauth2_provider_app_token_api_key;

DROP TABLE oauth2_provider_app_tokens;
DROP TABLE oauth2_provider_app_codes;

-- It is not possible to drop enum values from enum types, so the UP on
-- login_type has "IF NOT EXISTS".

-- The constraints on the secret prefix (which is used as an id embedded in the
-- secret) are dropped, but avoid completely reverting back to the previous
-- behavior since that will render existing secrets unusable once upgraded
-- again.  OAuth2 is blocked outside of development mode in previous versions,
-- so users will not be able to create broken secrets.  This is really just to
-- make sure tests keep working (say for a bisect).
ALTER TABLE ONLY oauth2_provider_app_secrets
    DROP CONSTRAINT oauth2_provider_app_secrets_secret_prefix_key,
    ALTER COLUMN secret_prefix DROP NOT NULL;
