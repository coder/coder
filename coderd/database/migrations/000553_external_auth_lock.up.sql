ALTER TABLE external_auth_links ADD COLUMN IF NOT EXISTS refresh_lease_expires_at timestamp WITH time zone DEFAULT NULL;
COMMENT ON COLUMN external_auth_links.refresh_lease_expires_at IS 'Indicates a replica is refreshing the token; prevents concurrent refreshes.';
