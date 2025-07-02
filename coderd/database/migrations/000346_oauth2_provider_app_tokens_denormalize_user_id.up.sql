-- Add user_id column to oauth2_provider_app_tokens for performance optimization
-- This eliminates the need to join with api_keys table for authorization checks
ALTER TABLE oauth2_provider_app_tokens
    ADD COLUMN user_id uuid;

-- Backfill existing records with user_id from the associated api_key
UPDATE oauth2_provider_app_tokens
SET user_id = api_keys.user_id
FROM api_keys
WHERE oauth2_provider_app_tokens.api_key_id = api_keys.id;

-- Make user_id NOT NULL after backfilling
ALTER TABLE oauth2_provider_app_tokens
    ALTER COLUMN user_id SET NOT NULL;

-- Add foreign key constraint to maintain referential integrity
ALTER TABLE oauth2_provider_app_tokens
    ADD CONSTRAINT fk_oauth2_provider_app_tokens_user_id
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE;

COMMENT ON COLUMN oauth2_provider_app_tokens.user_id IS 'Denormalized user ID for performance optimization in authorization checks';