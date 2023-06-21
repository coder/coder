BEGIN;

CREATE TABLE IF NOT EXISTS oauth_merge_state (
    state_string text NOT NULL,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    from_login_type login_type NOT NULL,
	to_login_type login_type NOT NULL,
    user_id uuid NOT NULL
        REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (state_string)
);

COMMENT ON TABLE oauth_merge_state IS 'Stores the state string for Oauth merge requests. If an Oauth state string is found in this table, '
    'it is assumed the user had a LoginType "password" and is switching to an Oauth based authentication.';

COMMENT ON COLUMN oauth_merge_state.expires_at IS 'The time at which the state string expires, a merge request times out if the user does not perform it quick enough.';

COMMENT ON COLUMN oauth_merge_state.to_login_type IS 'The login type the user is converting to. Should be github or oidc.';

COMMIT;


-- This has to be outside a transaction
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'convert_login';
