-- It's difficult to generate tokens for existing proxies, so we'll just delete
-- them if they exist.
--
-- No one is using this feature yet as of writing this migration, so this is
-- fine.
DELETE FROM workspace_proxies;

ALTER TABLE workspace_proxies
	ADD COLUMN token_hashed_secret bytea NOT NULL;

COMMENT ON COLUMN workspace_proxies.token_hashed_secret
	IS 'Hashed secret is used to authenticate the workspace proxy using a session token.';

COMMENT ON COLUMN workspace_proxies.deleted
	IS 'Boolean indicator of a deleted workspace proxy. Proxies are soft-deleted.';

COMMENT ON COLUMN workspace_proxies.icon
	IS 'Expects an emoji character. (/emojis/1f1fa-1f1f8.png)';
