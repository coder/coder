-- Restores the column and the type. It cannot restore the rows: the AI agent
-- users rows are gone and the ledger does not hold what they contained beyond
-- the identifier. `tigre` is a proof of concept branch with no deployment to
-- migrate down.

ALTER TABLE users
    DROP CONSTRAINT users_service_account_login_type,
    DROP CONSTRAINT users_email_not_empty;

CREATE TYPE user_kind AS ENUM (
    'human',
    'ai_agent'
);

ALTER TABLE users
    ADD COLUMN kind user_kind DEFAULT 'human'::user_kind NOT NULL;

ALTER TABLE aibridge_interceptions
    ADD CONSTRAINT aibridge_interceptions_initiator_id_fkey
        FOREIGN KEY (initiator_id) REFERENCES users (id);

ALTER TABLE users
    ADD CONSTRAINT users_email_not_empty
        CHECK ((email = '') = ((is_service_account = true) OR (kind = 'ai_agent'::user_kind))),
    ADD CONSTRAINT users_service_account_login_type
        CHECK ((((is_service_account = false) AND (kind <> 'ai_agent'::user_kind)) OR (login_type = 'none'::login_type)));
