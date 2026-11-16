INSERT INTO workspace_agent_relays (
	workspace_agent_id,
	public_key,
	generation,
	created_at,
	renewed_at
)
SELECT
	id,
	'\x00'::bytea,
	1,
	'2024-11-02 13:10:00.000000+02',
	'2024-11-02 13:10:00.000000+02'
FROM
	workspace_agents
ORDER BY
	id
LIMIT 1;
