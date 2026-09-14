INSERT INTO
	custom_roles (
	name,
	display_name,
	site_permissions,
	org_permissions,
	user_permissions,
	created_at,
	updated_at
)
VALUES
	(
	 	'custom-role',
	 	'Custom Role',
	 	'[{"negate":false,"resource_type":"deployment_config","action":"update"},{"negate":false,"resource_type":"workspace","action":"read"}]',
	 	'{}',
	 	'[{"negate":false,"resource_type":"workspace","action":"read"}]',
		date_trunc('hour', NOW()),
		date_trunc('hour', NOW()) + '30 minute'::interval
	);
