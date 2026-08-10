-- Reverse of up-step 4: restore the original NOT NULL. Fails if any
-- public-client token (app_secret_id IS NULL) exists. Revoke every
-- outstanding public-client session before rolling this migration back.
ALTER TABLE oauth2_provider_app_tokens ALTER COLUMN app_secret_id SET NOT NULL;

-- Reverse of up-step 3/1: drop the new column and its FK entirely.
ALTER TABLE oauth2_provider_app_tokens DROP CONSTRAINT oauth2_provider_app_tokens_app_id_fkey;
ALTER TABLE oauth2_provider_app_tokens DROP COLUMN app_id;
