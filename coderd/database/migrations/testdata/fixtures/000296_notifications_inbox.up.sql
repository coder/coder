INSERT INTO
	inbox_notifications (
		id,
		user_id,
		template_id,
		targets,
		title,
		content,
		icon,
		actions,
		read_at,
		created_at
	)
	VALUES (
		'68b396aa-7f53-4bf1-b8d8-4cbf5fa244e5', -- uuid
		'5755e622-fadd-44ca-98da-5df070491844', -- uuid
		'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', -- uuid
        ARRAY[]::UUID[], -- uuid[]
		'Test Notification',
		'This is a test notification',
		'https://test.coder.com/favicon.ico',
		'{}',
		'2025-01-01 00:00:00',
		'2025-01-01 00:00:00'
	);
