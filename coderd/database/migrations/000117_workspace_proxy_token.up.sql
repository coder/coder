BEGIN;

-- It's difficult to generate tokens for existing proxies, so we'll just delete
-- them if they exist.
--
-- No one is using this feature yet as of writing this migration, so this is
-- fine.
DELETE FROM workspace_proxies;

ALTER TABLE workspace_proxies
	ADD COLUMN token_hashed_secret bytea NOT NULL;

COMMIT;
