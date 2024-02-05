ALTER TABLE user_links ADD COLUMN debug_context jsonb DEFAULT '{}' NOT NULL;
COMMENT ON COLUMN user_links.debug_context IS 'Debug information includes information like id_token and userinfo claims.';
