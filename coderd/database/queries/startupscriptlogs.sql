-- name: GetStartupScriptLogsByJobID :many
SELECT
	*
FROM
	startup_script_logs
WHERE
	job_id = $1;

-- name: InsertOrUpdateStartupScriptLog :exec
INSERT INTO
	startup_script_logs (agent_id, job_id, output)
VALUES ($1, $2, $3)
ON CONFLICT (agent_id, job_id) DO UPDATE
	SET
		output = $3
	WHERE
		startup_script_logs.agent_id = $1
		AND startup_script_logs.job_id = $2;
