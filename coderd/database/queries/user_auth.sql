-- name: GetUserAuthByUserID :one
SELECT
  *
FROM
  user_auth
WHERE
  user_id = $1;
-- name: InsertUserAuth :one
INSERT INTO
  user_auth (
    user_id,
    login_type,
    linked_id
  )
VALUES
  ( $1, $2, $3) RETURNING *;
-- name: GetUserAuthByLinkedID :one
SELECT
  *
FROM
  user_auth
WHERE
  linked_id = $1;

