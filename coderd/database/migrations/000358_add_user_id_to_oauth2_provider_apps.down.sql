-- Restore refresh_hash as NOT NULL (existing data should still be valid)
ALTER TABLE oauth2_provider_app_tokens
ALTER COLUMN refresh_hash SET NOT NULL;

-- Remove user_id column from OAuth2 provider apps
ALTER TABLE oauth2_provider_apps
DROP COLUMN user_id;
