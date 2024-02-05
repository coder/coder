-- This migration makes no attempt to try to populate
-- the oauth_access_token, oauth_refresh_token, and oauth_expiry
-- columns of api_key rows with the values from the dropped user_links
-- table.
DROP TABLE IF EXISTS user_links;

ALTER TABLE
  api_keys
ADD COLUMN oauth_access_token text DEFAULT ''::text NOT NULL;

ALTER TABLE
  api_keys
ADD COLUMN oauth_refresh_token text DEFAULT ''::text NOT NULL;

ALTER TABLE
  api_keys
ADD COLUMN oauth_expiry timestamp with time zone DEFAULT '0001-01-01 00:00:00+00'::timestamp with time zone NOT NULL;

ALTER TABLE users DROP COLUMN login_type;
