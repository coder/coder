-- Default lifetime is 24hours.
ALTER TABLE api_keys ADD COLUMN lifetime_seconds bigint default 86400 NOT NULL;
