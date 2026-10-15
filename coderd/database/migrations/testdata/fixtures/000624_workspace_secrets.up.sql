INSERT INTO workspace_secrets (
	id,
	workspace_id,
	workspace_build_id,
	name,
	value,
	env_name,
	file_path,
	ephemeral,
	source
)
VALUES (
	'5b1e9d0c-2f3a-4c6e-9a7b-8d4f1e2c3b5a',
	'3a9a1feb-e89d-457c-9d53-ac751b198ebe',
	'a8c0b8c5-c9a8-4f33-93a4-8142e6858244',
	'secret-name',
	'secret value',
	'SECRET_ENV_NAME',
	'~/secret/file/path',
	false,
	'request'
);
