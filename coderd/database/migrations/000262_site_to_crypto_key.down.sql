-- Step 1: Remove the new entries from crypto_keys table
DELETE FROM crypto_keys
WHERE feature IN ('workspace_apps_token', 'workspace_apps_api_key', 'tailnet_resume')
  AND sequence = 1;

CREATE TYPE crypto_key_feature_old AS ENUM (
    'workspace_apps',
    'oidc_convert',
    'tailnet_resume'
);

ALTER TABLE crypto_keys
    ALTER COLUMN feature TYPE crypto_key_feature_old
    USING (feature::text::crypto_key_feature_old);

DROP TYPE crypto_key_feature;

ALTER TYPE crypto_key_feature_old RENAME TO crypto_key_feature;

