BEGIN;

CREATE TABLE IF NOT EXISTS oidc_merge_state (
    state_string text NOT NULL,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    user_id uuid NOT NULL
        REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (state_string)
);

COMMENT ON TABLE oidc_merge_state IS 'Stores the state string for OIDC merge requests. If an OIDC state string is found in this table, '
    'it is assumed the user had a LoginType "password" and is switching to an OIDC based authentication.';

COMMENT ON COLUMN oidc_merge_state.expires_at IS 'The time at which the state string expires, a merge request times out if the user does not perform it quick enough.';

COMMIT;
