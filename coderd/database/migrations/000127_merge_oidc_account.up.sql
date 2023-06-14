BEGIN;

CREATE TABLE IF NOT EXISTS oauth_merge_state (
    state_string text NOT NULL,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    oauth_id text NOT NULL,
    user_id uuid NOT NULL
        REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (state_string)
);

COMMENT ON TABLE oauth_merge_state IS 'Stores the state string for Oauth merge requests. If an Oauth state string is found in this table, '
    'it is assumed the user had a LoginType "password" and is switching to an Oauth based authentication.';

COMMENT ON COLUMN oauth_merge_state.expires_at IS 'The time at which the state string expires, a merge request times out if the user does not perform it quick enough.';

COMMENT ON COLUMN oauth_merge_state.oauth_id IS 'Identifier to know which Oauth provider the user is merging with. '
    'This prevents the user from requesting "github" and merging with a different Oauth provider'
;

COMMIT;
