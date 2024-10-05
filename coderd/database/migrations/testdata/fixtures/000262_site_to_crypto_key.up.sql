INSERT INTO site_configs (key, value)
VALUES (
  'app_signing_key',
  encode(gen_random_bytes(96), 'hex')
)
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value;

INSERT INTO site_configs (key, value)
VALUES (
  'coordinator_resume_token_signing_key',
  encode(gen_random_bytes(32), 'hex')
)
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value;
