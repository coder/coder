ALTER TABLE external_auth_links DROP COLUMN IF EXISTS refresh_lease_expires_at;
DROP FUNCTION IF EXISTS acquire_external_auth_link_refresh_lease;
