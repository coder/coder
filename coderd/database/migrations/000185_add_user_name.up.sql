ALTER TABLE users ADD COLUMN name text NOT NULL DEFAULT '';

COMMENT ON COLUMN users.name IS 'Name of the Coder user';

