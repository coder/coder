ALTER TABLE user_links RENAME COLUMN debug_context TO claims;

COMMENT ON COLUMN user_links.claims IS 'Claims from the IDP for the linked user. Includes both id_token and userinfo claims. ';
