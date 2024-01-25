-- name: GetJFrogXrayScanByWorkspaceAndAgentID :one
SELECT
	*
FROM
	jfrog_xray_scans
WHERE
	agent_id = $1
AND
	workspace_id = $2
LIMIT
	1;

-- name: UpsertJFrogXrayScanByWorkspaceAndAgentID :exec
INSERT INTO 
	jfrog_xray_scans (
		agent_id,
		workspace_id,
		payload
	)
VALUES 
	($1, $2, $3)
ON CONFLICT (agent_id, workspace_id)
DO UPDATE SET payload = $3;
