INSERT INTO
	ai_seat_state (
		user_id,
		first_used_at,
		last_used_at,
		last_event_type,
		last_event_description,
		updated_at
	)
VALUES
	('30095c71-380b-457a-8995-97b8ee6e5307', NOW(), NOW(), 'task'::ai_seat_usage_reason, 'Used for AI task', NOW());
