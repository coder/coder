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
		critical,
		high,
		medium,
		results_url
	)
VALUES 
	($1, $2, $3, $4, $5, $6)
ON CONFLICT (agent_id, workspace_id)
DO UPDATE SET critical = $3, high = $4, medium = $5, results_url = $6;
