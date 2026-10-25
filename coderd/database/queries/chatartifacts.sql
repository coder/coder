-- name: InsertChatArtifact :one
INSERT INTO chat_artifacts (id, organization_id, owner_id, project_id, title)
VALUES (
    COALESCE(sqlc.narg('id')::uuid, gen_random_uuid()),
    @organization_id::uuid,
    @owner_id::uuid,
    sqlc.narg('project_id')::uuid,
    @title::text
)
RETURNING *;

-- name: InsertChatArtifactVersion :one
-- Inserts base_version + 1 only while base_version is the latest version.
-- If two chats publish on the same base, the primary key refuses the second.
INSERT INTO chat_artifact_versions
    (artifact_id, version, label, name, mimetype, data, created_by, chat_id)
SELECT
    @artifact_id::uuid,
    (sqlc.arg('base_version')::int + 1)::int,
    @label::text,
    @name::text,
    @mimetype::text,
    @data::bytea,
    sqlc.narg('created_by')::uuid,
    sqlc.narg('chat_id')::uuid
WHERE sqlc.arg('base_version')::int = (
    SELECT COALESCE(MAX(version), 0)
    FROM chat_artifact_versions
    WHERE artifact_id = @artifact_id::uuid
)
RETURNING artifact_id, version, created_at;

-- name: UpdateChatArtifactOnPublish :one
-- Marks a publish. An empty title keeps the current title.
UPDATE chat_artifacts
SET
    title = COALESCE(NULLIF(@title::text, ''), title),
    updated_at = now()
WHERE id = @id::uuid
RETURNING *;

-- name: GetChatArtifactByID :one
SELECT
    sqlc.embed(chat_artifacts),
    COALESCE((
        SELECT MAX(chat_artifact_versions.version)
        FROM chat_artifact_versions
        WHERE chat_artifact_versions.artifact_id = chat_artifacts.id
    ), 0)::int AS latest_version,
    visible_users.username AS owner_username,
    COALESCE(chat_projects.name, '')::text AS project_name
FROM chat_artifacts
JOIN visible_users ON visible_users.id = chat_artifacts.owner_id
LEFT JOIN chat_projects ON chat_projects.id = chat_artifacts.project_id
WHERE chat_artifacts.id = @id::uuid;

-- name: GetChatArtifactVersion :one
SELECT *
FROM chat_artifact_versions
WHERE artifact_id = @artifact_id::uuid
    AND version = @version::int;

-- name: GetLatestChatArtifactVersion :one
SELECT *
FROM chat_artifact_versions
WHERE artifact_id = @artifact_id::uuid
ORDER BY version DESC
LIMIT 1;

-- name: GetChatArtifactVersionsMetadata :many
-- Lists the versions of an artifact, newest first, without their bytes.
SELECT
    chat_artifact_versions.artifact_id,
    chat_artifact_versions.version,
    chat_artifact_versions.label,
    chat_artifact_versions.name,
    chat_artifact_versions.mimetype,
    length(chat_artifact_versions.data)::bigint AS size,
    chat_artifact_versions.created_by,
    COALESCE(visible_users.username, '')::text AS created_by_username,
    chat_artifact_versions.chat_id,
    COALESCE(chats.title, '')::text AS chat_title,
    chats.owner_id AS chat_owner_id,
    chat_artifact_versions.created_at
FROM chat_artifact_versions
LEFT JOIN visible_users ON visible_users.id = chat_artifact_versions.created_by
LEFT JOIN chats ON chats.id = chat_artifact_versions.chat_id
WHERE chat_artifact_versions.artifact_id = @artifact_id::uuid
ORDER BY chat_artifact_versions.version DESC;

-- name: GetChatArtifactsOwnedOrInUserProjects :many
-- Lists the artifacts that the user owns or can reach through a chat
-- project, with the same membership rule as IsChatProjectUsableByUserID.
-- Callers must still authorize each row.
SELECT
    sqlc.embed(chat_artifacts),
    COALESCE((
        SELECT MAX(chat_artifact_versions.version)
        FROM chat_artifact_versions
        WHERE chat_artifact_versions.artifact_id = chat_artifacts.id
    ), 0)::int AS latest_version,
    visible_users.username AS owner_username,
    COALESCE(chat_projects.name, '')::text AS project_name
FROM chat_artifacts
JOIN visible_users ON visible_users.id = chat_artifacts.owner_id
LEFT JOIN chat_projects ON chat_projects.id = chat_artifacts.project_id
WHERE chat_artifacts.owner_id = @user_id::uuid
    OR chat_projects.owner_id = @user_id::uuid
    OR (
        EXISTS (
            SELECT 1
            FROM organization_members
            WHERE organization_members.user_id = @user_id::uuid
                AND organization_members.organization_id = chat_projects.organization_id
        )
        AND (
            chat_projects.user_acl -> (@user_id::uuid)::text -> 'permissions' ?| ARRAY['read', '*']
            OR chat_projects.group_acl -> chat_projects.organization_id::text -> 'permissions' ?| ARRAY['read', '*']
            OR EXISTS (
                SELECT 1
                FROM group_members
                WHERE group_members.user_id = @user_id::uuid
                    AND chat_projects.group_acl -> group_members.group_id::text -> 'permissions' ?| ARRAY['read', '*']
            )
        )
    )
ORDER BY chat_artifacts.updated_at DESC, chat_artifacts.id;
