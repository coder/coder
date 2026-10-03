-- Public (secretless, PKCE-only) OAuth2 clients have no client_secret, so
-- their tokens have nothing to put in app_secret_id. Add a direct app_id
-- column so token ownership checks (e.g. revocation) don't have to join
-- through a secret that may not exist, then loosen app_secret_id's NOT NULL.

-- Step 1: add app_id as nullable first.
ALTER TABLE oauth2_provider_app_tokens ADD COLUMN app_id uuid;

-- Step 2: backfill every existing row via the only path available today
-- (the same join revoke.go currently does at request time).
UPDATE oauth2_provider_app_tokens t
SET app_id = s.app_id
FROM oauth2_provider_app_secrets s
WHERE t.app_secret_id = s.id;

-- Step 3: now that every row has a value, constrain it.
ALTER TABLE oauth2_provider_app_tokens ALTER COLUMN app_id SET NOT NULL;
ALTER TABLE oauth2_provider_app_tokens
    ADD CONSTRAINT oauth2_provider_app_tokens_app_id_fkey
    FOREIGN KEY (app_id) REFERENCES oauth2_provider_apps(id) ON DELETE CASCADE;

-- Step 4: only now loosen app_secret_id, since every row already has a
-- reliable app_id to fall back on before this runs.
ALTER TABLE oauth2_provider_app_tokens ALTER COLUMN app_secret_id DROP NOT NULL;

COMMENT ON COLUMN oauth2_provider_app_tokens.app_id IS 'Denormalized app ID so ownership checks (e.g. revocation) do not need to join through app_secret_id, which is NULL for public clients.';
