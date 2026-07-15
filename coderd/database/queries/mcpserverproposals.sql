-- name: InsertMCPServerProposal :one
INSERT INTO mcp_server_proposals (
    id,
    chat_id,
    requester_id,
    channel,
    thread_ts,
    message_ts,
    request
) VALUES (
    @id::uuid,
    @chat_id::uuid,
    @requester_id::uuid,
    @channel::text,
    @thread_ts::text,
    @message_ts::text,
    @request::jsonb
)
RETURNING
    *;

-- name: GetMCPServerProposalByID :one
SELECT
    *
FROM
    mcp_server_proposals
WHERE
    id = @id::uuid;

-- name: GetMCPServerProposalByIDForUpdate :one
-- Locks the proposal row so concurrent accept/reject transitions across
-- coderd replicas serialize on the row lock.
SELECT
    *
FROM
    mcp_server_proposals
WHERE
    id = @id::uuid
FOR UPDATE;

-- name: UpdateMCPServerProposalStatus :one
UPDATE
    mcp_server_proposals
SET
    status = @status::text,
    mcp_server_config_id = sqlc.narg('mcp_server_config_id')::uuid,
    accepted_at = sqlc.narg('accepted_at')::timestamptz
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: DeleteExpiredMCPServerProposals :exec
DELETE FROM
    mcp_server_proposals
WHERE
    created_at < @created_before::timestamptz;
