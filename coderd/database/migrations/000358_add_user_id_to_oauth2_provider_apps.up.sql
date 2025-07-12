-- Add user ownership to OAuth2 provider apps for client credentials support
ALTER TABLE oauth2_provider_apps
ADD COLUMN user_id uuid REFERENCES users(id) ON DELETE CASCADE;

-- Make refresh_hash nullable to support client credentials tokens
-- RFC 6749 Section 4.4.3: "A refresh token SHOULD NOT be included" for client credentials
ALTER TABLE oauth2_provider_app_tokens
ALTER COLUMN refresh_hash DROP NOT NULL;
