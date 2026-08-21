CREATE TABLE valid_credentials (
	actor_type text NOT NULL,
	actor uuid NOT NULL,
	password text NOT NULL
);

CREATE INDEX idx_valid_credentials_actor ON valid_credentials (actor);
