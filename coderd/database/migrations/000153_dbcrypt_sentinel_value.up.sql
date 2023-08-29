CREATE TABLE IF NOT EXISTS dbcrypt_sentinel (
		only_one integer GENERATED ALWAYS AS (1) STORED UNIQUE,
		val text NOT NULL DEFAULT ''::text
);

COMMENT ON TABLE dbcrypt_sentinel IS 'A table used to determine if the database is encrypted';
COMMENT ON COLUMN dbcrypt_sentinel.only_one IS 'Ensures that only one row exists in the table.';
COMMENT ON COLUMN dbcrypt_sentinel.val IS 'Used to determine if the database is encrypted.';
