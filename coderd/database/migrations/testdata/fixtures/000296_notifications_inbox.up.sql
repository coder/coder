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
		'45e89705-e09d-4850-bcec-f9a937f5d78d', -- uuid
		'193590e9-918f-4ef9-be47-04625f49c4c3', -- uuid
        ARRAY[]::UUID[], -- uuid[]
		'Test Notification',
		'This is a test notification',
		'https://test.coder.com/favicon.ico',
		'{}',
		'2024-01-01 00:00:00',
		'2024-01-01 00:00:00'
	);
