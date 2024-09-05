-- name: GetFrobulators :many
SELECT *
FROM frobulators
WHERE user_id = $1 AND org_id = $2;

-- name: InsertFrobulator :one
INSERT INTO frobulators (id, user_id, org_id, model_number)
VALUES (gen_random_uuid(), $1, $2, $3)
RETURNING *;

-- name: DeleteFrobulator :exec
DELETE FROM frobulators
WHERE id = $1 AND user_id = $2 AND org_id = $3;