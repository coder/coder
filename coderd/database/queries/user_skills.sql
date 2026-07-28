-- name: InsertUserSkill :one
INSERT INTO user_skills (id, user_id, name, description, content)
VALUES (@id::uuid, @user_id::uuid, @name::text, @description::text, @content::text)
RETURNING *;

-- name: GetUserSkillByUserIDAndName :one
SELECT *
FROM user_skills
WHERE user_id = @user_id AND name = @name;

-- name: ListUserSkillMetadataByUserID :many
SELECT
    id, user_id, name, description, created_at, updated_at
FROM user_skills
WHERE user_id = @user_id
ORDER BY name ASC;

-- name: UpdateUserSkillByUserIDAndName :one
UPDATE user_skills
SET
    description = @description,
    content     = @content,
    updated_at  = now()
WHERE user_id = @user_id AND name = @name
RETURNING *;

-- name: DeleteUserSkillByUserIDAndName :one
DELETE FROM user_skills
WHERE user_id = @user_id AND name = @name
RETURNING *;
