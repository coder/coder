-- Add oauth_id_token column to user_links table to support ID token storage for OIDC providers like Azure
ALTER TABLE user_links ADD COLUMN oauth_id_token text DEFAULT ''::text NOT NULL;
