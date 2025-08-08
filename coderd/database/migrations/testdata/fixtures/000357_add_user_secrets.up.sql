INSERT INTO user_secrets (
	id,
	user_id,
	name,
	description,
	value,
	env_name,
	file_path
)
VALUES (
	'4848b19e-b392-4a1b-bc7d-0b7ffb41ef87',
	'30095c71-380b-457a-8995-97b8ee6e5307',
	'secret-name',
	'secret description',
	'secret value',
	'SECRET_ENV_NAME',
	'~/secret/file/path'
);
