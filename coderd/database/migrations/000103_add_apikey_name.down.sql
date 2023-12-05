DROP INDEX idx_api_key_name;

ALTER TABLE ONLY api_keys
  DROP COLUMN IF EXISTS token_name;
