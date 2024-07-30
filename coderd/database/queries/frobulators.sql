-- name: GetUserFrobulators :many
SELECT *
FROM frobulators
WHERE user_id = @user_id::uuid;

-- name: GetAllFrobulators :many
SELECT *
FROM frobulators;

-- name: InsertFrobulator :exec
INSERT INTO frobulators (id, user_id, model_number)
VALUES ($1, $2, $3);