CREATE TYPE ai_seat_usage_reason AS ENUM (
	'aibridge',
	'task'
);

CREATE TABLE ai_seat_state (
	user_id uuid NOT NULL PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
	first_used_at timestamptz NOT NULL,
	last_used_at timestamptz NOT NULL,
	last_event_type ai_seat_usage_reason NOT NULL,
	last_event_description text NOT NULL,
	updated_at timestamptz NOT NULL
);
