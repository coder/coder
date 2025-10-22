CREATE TABLE oauth2_provider_apps (
    id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    name varchar(64) NOT NULL,
    icon varchar(256) NOT NULL,
    callback_url text NOT NULL,
    PRIMARY KEY (id),
    UNIQUE(name)
);

COMMENT ON TABLE oauth2_provider_apps IS 'A table used to configure apps that can use Coder as an OAuth2 provider, the reverse of what we are calling external authentication.';

CREATE TABLE oauth2_provider_app_secrets (
    id uuid NOT NULL,
    created_at timestamp with time zone NOT NULL,
    last_used_at timestamp with time zone NULL,
    hashed_secret bytea NOT NULL,
    display_secret text NOT NULL,
    app_id uuid NOT NULL REFERENCES oauth2_provider_apps (id) ON DELETE CASCADE,
    PRIMARY KEY (id),
    UNIQUE(app_id, hashed_secret)
);

COMMENT ON COLUMN oauth2_provider_app_secrets.display_secret IS 'The tail end of the original secret so secrets can be differentiated.';
