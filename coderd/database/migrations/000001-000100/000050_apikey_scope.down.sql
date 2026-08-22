-- Avoid "upgrading" devurl keys to fully fledged API keys.
DELETE FROM api_keys WHERE scope != 'all';

ALTER TABLE api_keys DROP COLUMN scope;

DROP TYPE api_key_scope;
