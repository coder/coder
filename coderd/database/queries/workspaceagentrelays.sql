-- ClaimWorkspaceAgentRelay lets a daemon instance take or keep the relay of
-- its workspace agent. The instance that holds the relay renews it. Another
-- instance takes it over only when the holder has not renewed since
-- lapsed_before, and the generation goes up. It returns no row while another
-- instance holds the relay.
-- name: ClaimWorkspaceAgentRelay :one
INSERT INTO workspace_agent_relays (
	workspace_agent_id,
	public_key,
	generation,
	created_at,
	renewed_at
)
VALUES (
	@workspace_agent_id,
	@public_key,
	1,
	@now,
	@now
)
ON CONFLICT (workspace_agent_id) DO UPDATE SET
	generation = CASE
		WHEN workspace_agent_relays.public_key = EXCLUDED.public_key THEN workspace_agent_relays.generation
		ELSE workspace_agent_relays.generation + 1
	END,
	public_key = EXCLUDED.public_key,
	renewed_at = EXCLUDED.renewed_at
WHERE
	workspace_agent_relays.public_key = EXCLUDED.public_key
	OR workspace_agent_relays.renewed_at < @lapsed_before
RETURNING *;

-- name: GetWorkspaceAgentRelayByAgentID :one
SELECT
	*
FROM
	workspace_agent_relays
WHERE
	workspace_agent_id = @workspace_agent_id;

-- ReleaseWorkspaceAgentRelay gives the relay up, so that the next daemon
-- instance does not have to wait for the lease to lapse. Only the holder
-- can release it. The row stays, so that the generation keeps counting.
-- name: ReleaseWorkspaceAgentRelay :exec
UPDATE
	workspace_agent_relays
SET
	renewed_at = to_timestamp(0)
WHERE
	workspace_agent_id = @workspace_agent_id
	AND public_key = @public_key;
