-- The scope negotiated at /oauth2/authorize travels with the grant itself:
-- recorded on the code when it is issued, then carried onto the token it is
-- exchanged for so a refresh can narrow against what was actually granted
-- rather than against the app's current allowlist.
--
-- Both columns are nullable with no backfill. A NULL means "no scope was
-- recorded for this grant", which the token endpoint reads as unrestricted
-- access, so codes and tokens issued before this migration keep working.

ALTER TABLE oauth2_provider_app_codes ADD COLUMN scope text;

ALTER TABLE oauth2_provider_app_tokens ADD COLUMN scope text;

COMMENT ON COLUMN oauth2_provider_app_codes.scope IS 'Space-separated scope negotiated at authorization time. NULL means no scope was recorded and the exchanged token is unrestricted.';

COMMENT ON COLUMN oauth2_provider_app_tokens.scope IS 'Space-separated scope granted to this token. A refresh may narrow this but never widen it. NULL means no scope was recorded and the token is unrestricted.';
