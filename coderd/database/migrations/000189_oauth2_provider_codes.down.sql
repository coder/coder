DROP TRIGGER IF EXISTS trigger_delete_oauth2_provider_app_token ON oauth2_provider_app_tokens;
DROP FUNCTION IF EXISTS delete_deleted_oauth2_provider_app_token_api_key;

DROP TABLE oauth2_provider_app_tokens;
DROP TABLE oauth2_provider_app_codes;

-- It is not possible to drop enum values from enum types, so the UP on
-- login_type has "IF NOT EXISTS".
