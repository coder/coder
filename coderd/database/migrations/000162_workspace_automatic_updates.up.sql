BEGIN;
-- making this an enum in case we want to later add other options, like 'if_compatible_vars'
CREATE TYPE automatic_updates AS ENUM (
    'always',
    'never'
);
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS automatic_updates automatic_updates NOT NULL DEFAULT 'never'::automatic_updates;
COMMIT;
