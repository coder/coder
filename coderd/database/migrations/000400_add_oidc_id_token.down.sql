-- Remove oauth_id_token column from user_links table
ALTER TABLE user_links DROP COLUMN oauth_id_token;
