-- name: GetUserAISeatStates :many
-- Returns user IDs from the provided list that are consuming an AI seat.
-- Filters to active, non-deleted, non-system users to match the canonical
-- seat count query (GetActiveAISeatCount).
SELECT
	ais.user_id
FROM
	ai_seat_state ais
JOIN
	users u
ON
	ais.user_id = u.id
WHERE
	ais.user_id = ANY(@user_ids::uuid[])
	AND u.status = 'active'::user_status
	AND u.deleted = false
	AND u.is_system = false;
