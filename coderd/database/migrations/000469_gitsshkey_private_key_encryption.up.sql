ALTER TABLE gitsshkeys
ADD COLUMN IF NOT EXISTS private_key_key_id text REFERENCES dbcrypt_keys(active_key_digest);
