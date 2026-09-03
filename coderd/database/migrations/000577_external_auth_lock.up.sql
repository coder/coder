ALTER TABLE external_auth_links ADD COLUMN IF NOT EXISTS refresh_lease_expires_at timestamp WITH time zone DEFAULT NULL;
COMMENT ON COLUMN external_auth_links.refresh_lease_expires_at IS 'Indicates a replica is refreshing the token; prevents concurrent refreshes.';

CREATE OR REPLACE FUNCTION acquire_external_auth_link_refresh_lease(arg_provider_id text, arg_user_id uuid, timeout_ms bigint)
RETURNS SETOF external_auth_links AS $$
DECLARE r external_auth_links;
BEGIN
	UPDATE external_auth_links
	SET
		refresh_lease_expires_at = NOW() + (timeout_ms || ' ms')::interval
	WHERE
		provider_id = arg_provider_id
		AND user_id = arg_user_id
		AND (refresh_lease_expires_at IS NULL OR refresh_lease_expires_at < NOW())
	RETURNING * INTO r;
	-- Got the lease, return the one row.
	IF FOUND THEN
		RETURN NEXT r;
		RETURN;
	END IF;
	-- Differentiate between unable to get the lease and the row being gone.
	IF EXISTS (SELECT 1 FROM external_auth_links WHERE provider_id = arg_provider_id AND user_id = arg_user_id) THEN
		RAISE EXCEPTION 'row is currently leased by another replica'
			USING ERRCODE = 'check_violation',
				CONSTRAINT = 'external_auth_link_active_lease';
	END IF;
	-- Row is gone, return nothing.
	RETURN;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION acquire_external_auth_link_refresh_lease IS 'Acquire a lease on the external auth link and return the row. If there is already an active lease, an exception is raised.';
