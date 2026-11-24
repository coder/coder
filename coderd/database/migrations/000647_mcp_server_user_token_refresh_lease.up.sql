ALTER TABLE mcp_server_user_tokens ADD COLUMN refresh_lease_expires_at timestamp with time zone DEFAULT NULL;

COMMENT ON COLUMN mcp_server_user_tokens.refresh_lease_expires_at IS 'Indicates a replica is refreshing the token; prevents concurrent refreshes.';

CREATE FUNCTION acquire_mcp_server_user_token_refresh_lease(arg_id uuid, timeout_ms bigint)
RETURNS SETOF mcp_server_user_tokens AS $$
DECLARE r mcp_server_user_tokens;
BEGIN
	-- The lease never touches updated_at, so callers can keep using
	-- updated_at as the optimistic lock for the token material.
	UPDATE mcp_server_user_tokens
	SET
		refresh_lease_expires_at = NOW() + (timeout_ms || ' ms')::interval
	WHERE
		id = arg_id
		AND (refresh_lease_expires_at IS NULL OR refresh_lease_expires_at < NOW())
	RETURNING * INTO r;
	-- Got the lease, return the one row.
	IF FOUND THEN
		RETURN NEXT r;
		RETURN;
	END IF;
	-- Differentiate between unable to get the lease and the row being gone.
	IF EXISTS (SELECT 1 FROM mcp_server_user_tokens WHERE id = arg_id) THEN
		RAISE EXCEPTION 'row is currently leased by another replica'
			USING ERRCODE = 'check_violation',
				CONSTRAINT = 'mcp_server_user_token_active_lease';
	END IF;
	-- Row is gone, return nothing.
	RETURN;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION acquire_mcp_server_user_token_refresh_lease IS 'Acquire a lease on the MCP server user token and return the row. If there is already an active lease, an exception is raised.';
