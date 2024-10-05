-- Create a new enum type with the desired values
CREATE TYPE new_crypto_key_feature AS ENUM (
    'workspace_apps_token',
    'workspace_apps_api_key',
    'oidc_convert',
    'tailnet_resume'
);

-- Drop the old type and rename the new one
ALTER TABLE crypto_keys
    ALTER COLUMN feature TYPE new_crypto_key_feature
    USING (feature::text::new_crypto_key_feature);

DROP TYPE crypto_key_feature;

ALTER TYPE new_crypto_key_feature RENAME TO crypto_key_feature;

-- Extract and decode the app_signing_key, then insert the first 64 bytes for workspace_apps_token
INSERT INTO crypto_keys (feature, sequence, secret, secret_key_id, starts_at, deletes_at)
SELECT 
  'workspace_apps_token'::crypto_key_feature,
  1,
  encode(substring(decode(value, 'hex') from 1 for 64), 'base64'),
  NULL,
  '1970-01-01 00:00:00 UTC'::timestamptz,
  NULL
FROM site_configs
WHERE key = 'app_signing_key';

-- Extract and decode the app_signing_key, then insert the last 32 bytes for workspace_apps_api_key
INSERT INTO crypto_keys (feature, sequence, secret, secret_key_id, starts_at, deletes_at)
SELECT 
  'workspace_apps_api_key'::crypto_key_feature,
  1,
  encode(substring(decode(value, 'hex') from -32), 'base64'),
  NULL,
  '1970-01-01 00:00:00 UTC'::timestamptz,
  NULL
FROM site_configs
WHERE key = 'app_signing_key';

-- Extract and decode the coordinator_resume_token_signing_key, then insert it for tailnet_resume feature
INSERT INTO crypto_keys (feature, sequence, secret, secret_key_id, starts_at, deletes_at)
SELECT 
  'tailnet_resume'::crypto_key_feature,
  1,
  encode(decode(value, 'hex'), 'base64'),
  NULL,
  '1970-01-01 00:00:00 UTC'::timestamptz,
  NULL
FROM site_configs
WHERE key = 'coordinator_resume_token_signing_key';
