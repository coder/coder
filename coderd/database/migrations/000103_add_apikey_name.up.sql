ALTER TABLE ONLY api_keys
  ADD COLUMN IF NOT EXISTS token_name text NOT NULL DEFAULT '';

UPDATE
  api_keys
SET
  token_name = gen_random_uuid ()::text
WHERE
  login_type = 'token';

CREATE UNIQUE INDEX idx_api_key_name ON api_keys USING btree (user_id, token_name)
WHERE
  (login_type = 'token');
