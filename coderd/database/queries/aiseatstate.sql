-- name: GetUserAISeatStates :many
-- Returns user IDs from the provided list that have an entry in
-- ai_seat_state, meaning they are consuming an AI seat.
SELECT
	ais.user_id
FROM
	ai_seat_state ais
WHERE
	ais.user_id = ANY(@user_ids::uuid[]);
