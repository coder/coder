-- name: UpsertAISeatState :one
-- Returns true if a new rows was inserted, false otherwise.
INSERT INTO ai_seat_state (
	user_id,
	first_used_at,
	last_used_at,
	last_event_type,
	last_event_description,
	updated_at
)
VALUES
	($1, $2, $2, $3, $4, $2)
ON CONFLICT (user_id) DO UPDATE
SET
	last_used_at = EXCLUDED.last_used_at,
	last_event_type = EXCLUDED.last_event_type,
	last_event_description = EXCLUDED.last_event_description,
	updated_at = EXCLUDED.updated_at
RETURNING
	-- Postgres vodoo to know if a row was inserted.
	(xmax = 0)::boolean AS is_new;

-- name: GetActiveAISeatCount :one
SELECT
	COUNT(*)
FROM
	ai_seat_state ais
JOIN
	users u
ON
	ais.user_id = u.id
WHERE
	u.status = 'active'::user_status
	AND u.deleted = false
	AND u.is_system = false;
