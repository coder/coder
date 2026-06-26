ALTER TABLE gitsshkeys
    DROP CONSTRAINT gitsshkeys_private_key_key_id_fkey,
    DROP COLUMN private_key_key_id;
