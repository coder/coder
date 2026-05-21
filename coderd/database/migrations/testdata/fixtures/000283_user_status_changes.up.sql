INSERT INTO
	users (
		id,
		email,
		username,
		hashed_password,
		created_at,
		updated_at,
		status,
		rbac_roles,
		login_type,
		avatar_url,
		last_seen_at,
		quiet_hours_schedule,
		theme_preference,
		name,
		github_com_user_id,
		hashed_one_time_passcode,
		one_time_passcode_expires_at
	)
	VALUES (
		'5755e622-fadd-44ca-98da-5df070491844', -- uuid
		'test@example.com',
		'testuser',
		'hashed_password',
		'2024-01-01 00:00:00',
		'2024-01-01 00:00:00',
		'active',
		'{}',
		'password',
		'',
		'2024-01-01 00:00:00',
		'',
		'',
		'',
		123,
		NULL,
		NULL
	);

UPDATE users SET status = 'dormant', updated_at = '2024-01-01 01:00:00' WHERE id = '5755e622-fadd-44ca-98da-5df070491844';
UPDATE users SET deleted = true, updated_at = '2024-01-01 02:00:00' WHERE id = '5755e622-fadd-44ca-98da-5df070491844';
