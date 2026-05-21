ALTER TABLE user_links RENAME COLUMN claims TO debug_context;

COMMENT ON COLUMN user_links.debug_context IS 'Debug information includes information like id_token and userinfo claims.';
