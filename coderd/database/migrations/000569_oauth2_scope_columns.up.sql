-- The scope negotiated at /oauth2/authorize travels with the grant itself:
-- recorded on the code when it is issued, then carried onto the token it is
-- exchanged for, so a refresh can be narrowed against what was actually
-- granted rather than against the app's current allowlist.
--
-- Existing rows are unrestricted in fact rather than by omission, since
-- apikey.Generate mints every OAuth2 access key with the coder:all scope.
-- The backfill writes that down. Both columns are then NOT NULL with no
-- default, so a grant's authority is always stated explicitly and a caller
-- that omits the column fails instead of silently issuing full access.

ALTER TABLE oauth2_provider_app_codes ADD COLUMN scope text;

ALTER TABLE oauth2_provider_app_tokens ADD COLUMN scope text;

UPDATE oauth2_provider_app_codes SET scope = 'coder:all' WHERE scope IS NULL;

UPDATE oauth2_provider_app_tokens SET scope = 'coder:all' WHERE scope IS NULL;

ALTER TABLE oauth2_provider_app_codes
	ALTER COLUMN scope SET NOT NULL,
	ADD CONSTRAINT oauth2_provider_app_codes_scope_not_empty CHECK (scope <> '');

ALTER TABLE oauth2_provider_app_tokens
	ALTER COLUMN scope SET NOT NULL,
	ADD CONSTRAINT oauth2_provider_app_tokens_scope_not_empty CHECK (scope <> '');

COMMENT ON COLUMN oauth2_provider_app_codes.scope IS 'Space-separated scope negotiated at authorization time, drawn from the api_key_scope vocabulary. Always set; coder:all records an unrestricted grant.';

COMMENT ON COLUMN oauth2_provider_app_tokens.scope IS 'Space-separated scope granted to this token, drawn from the api_key_scope vocabulary. Always set; coder:all records an unrestricted grant. Later phases will narrow this on refresh and never widen it.';
