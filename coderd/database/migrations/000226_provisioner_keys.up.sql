CREATE TABLE provisioner_keys (
    id uuid PRIMARY KEY,
    created_at timestamptz NOT NULL,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name varchar(64) NOT NULL,
    hashed_secret bytea NOT NULL,
    UNIQUE (organization_id, name)
);
