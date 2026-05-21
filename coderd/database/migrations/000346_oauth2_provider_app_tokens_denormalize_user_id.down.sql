-- Remove the denormalized user_id column from oauth2_provider_app_tokens
ALTER TABLE oauth2_provider_app_tokens
    DROP CONSTRAINT IF EXISTS fk_oauth2_provider_app_tokens_user_id;

ALTER TABLE oauth2_provider_app_tokens
    DROP COLUMN IF EXISTS user_id;