CREATE TABLE oauth2_provider_app_codes (
    id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    secret_prefix bytea NOT NULL,
    hashed_secret bytea NOT NULL,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    app_id uuid NOT NULL REFERENCES oauth2_provider_apps (id) ON DELETE CASCADE,
    PRIMARY KEY (id),
    UNIQUE(secret_prefix)
);

COMMENT ON TABLE oauth2_provider_app_codes IS 'Codes are meant to be exchanged for access tokens.';

CREATE TABLE oauth2_provider_app_tokens (
    id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    hash_prefix bytea NOT NULL,
    refresh_hash bytea NOT NULL,
    app_secret_id uuid NOT NULL REFERENCES oauth2_provider_app_secrets (id) ON DELETE CASCADE,
    api_key_id text NOT NULL REFERENCES api_keys (id) ON DELETE CASCADE,
    PRIMARY KEY (id),
    UNIQUE(hash_prefix)
);

COMMENT ON COLUMN oauth2_provider_app_tokens.refresh_hash IS 'Refresh tokens provide a way to refresh an access token (API key). An expired API key can be refreshed if this token is not yet expired, meaning this expiry can outlive an API key.';

-- When we delete a token, delete the API key associated with it.
CREATE FUNCTION delete_deleted_oauth2_provider_app_token_api_key() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
BEGIN
    DELETE FROM api_keys
    WHERE id = OLD.api_key_id;
    RETURN OLD;
END;
$$;

CREATE TRIGGER trigger_delete_oauth2_provider_app_token
AFTER DELETE ON oauth2_provider_app_tokens
FOR EACH ROW
EXECUTE PROCEDURE delete_deleted_oauth2_provider_app_token_api_key();

ALTER TYPE login_type ADD VALUE IF NOT EXISTS 'oauth2_provider_app';

-- Switch to an ID we will prefix to the raw secret that we give to the user
-- (instead of matching on the entire secret as the ID, since they will be
-- salted and we can no longer do that).  OAuth2 is blocked outside of
-- development mode so there should be no production secrets unless they
-- previously upgraded, in which case they keep their original prefixes and will
-- be fine.  Add a random ID for the development mode case so the upgrade does
-- not fail, at least.
ALTER TABLE ONLY oauth2_provider_app_secrets
    ADD COLUMN IF NOT EXISTS secret_prefix bytea NULL;

UPDATE oauth2_provider_app_secrets
    SET secret_prefix = substr(md5(random()::text), 0, 10)::bytea
    WHERE secret_prefix IS NULL;

ALTER TABLE ONLY oauth2_provider_app_secrets
    ALTER COLUMN secret_prefix SET NOT NULL,
    ADD CONSTRAINT oauth2_provider_app_secrets_secret_prefix_key UNIQUE (secret_prefix),
    DROP CONSTRAINT IF EXISTS oauth2_provider_app_secrets_app_id_hashed_secret_key;
