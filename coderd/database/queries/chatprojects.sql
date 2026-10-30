-- name: InsertChatProject :one
INSERT INTO chat_projects (id, organization_id, owner_id, name, description, icon, agent_user_id, model_config_id)
VALUES (
    COALESCE(sqlc.narg('id')::uuid, gen_random_uuid()),
    @organization_id::uuid,
    @owner_id::uuid,
    @name::text,
    @description::text,
    @icon::text,
    sqlc.narg('agent_user_id')::uuid,
    sqlc.narg('model_config_id')::uuid
)
RETURNING *;

-- name: GetChatProjectByID :one
SELECT *
FROM chat_projects
WHERE id = @id::uuid;

-- name: GetChatProjectByIDForUpdate :one
SELECT *
FROM chat_projects
WHERE id = @id::uuid
FOR UPDATE;

-- name: GetChatProjectsByOwnerID :many
SELECT *
FROM chat_projects
WHERE owner_id = @owner_id::uuid
ORDER BY lower(name), id;

-- name: GetChatProjectsOwnedOrSharedWithUserID :many
-- Entries match regardless of the actions they grant, so callers must
-- authorize each row.
SELECT chat_projects.*
FROM chat_projects
WHERE chat_projects.owner_id = @user_id::uuid
    OR (
        (
            chat_projects.user_acl ? (@user_id::uuid)::text
            OR chat_projects.group_acl ?| ARRAY(
                SELECT group_members.group_id::text
                FROM group_members
                WHERE group_members.user_id = @user_id::uuid
                UNION ALL
                SELECT organization_members.organization_id::text
                FROM organization_members
                WHERE organization_members.user_id = @user_id::uuid
            )
        )
        AND EXISTS (
            SELECT 1
            FROM organization_members
            WHERE organization_members.user_id = @user_id::uuid
                AND organization_members.organization_id = chat_projects.organization_id
        )
    )
ORDER BY lower(chat_projects.name), chat_projects.id;

-- name: IsChatProjectAccessibleByUserID :one
SELECT EXISTS (
    SELECT 1
    FROM chat_projects
    WHERE chat_projects.id = @project_id::uuid
        AND (
            chat_projects.owner_id = @user_id::uuid
            OR chat_projects.agent_user_id = @user_id::uuid
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
        )
)::boolean;

-- name: IsChatProjectEditableByUserID :one
-- Reports whether a user owns a project or it is shared with them with
-- edit access. Agent users never edit projects.
SELECT EXISTS (
    SELECT 1
    FROM chat_projects
    WHERE chat_projects.id = @project_id::uuid
        AND (
            chat_projects.owner_id = @user_id::uuid
            OR (
                EXISTS (
                    SELECT 1
                    FROM organization_members
                    WHERE organization_members.user_id = @user_id::uuid
                        AND organization_members.organization_id = chat_projects.organization_id
                )
                AND (
                    chat_projects.user_acl -> (@user_id::uuid)::text -> 'permissions' ?| ARRAY['update', '*']
                    OR chat_projects.group_acl -> chat_projects.organization_id::text -> 'permissions' ?| ARRAY['update', '*']
                    OR EXISTS (
                        SELECT 1
                        FROM group_members
                        WHERE group_members.user_id = @user_id::uuid
                            AND chat_projects.group_acl -> group_members.group_id::text -> 'permissions' ?| ARRAY['update', '*']
                    )
                )
            )
        )
)::boolean;

-- name: UpdateChatProjectACLByID :exec
UPDATE chat_projects
SET
    user_acl = @user_acl,
    group_acl = @group_acl
WHERE id = @id::uuid;

-- name: UpdateChatProjectByID :one
UPDATE chat_projects
SET
    name = @name::text,
    description = @description::text,
    icon = @icon::text,
    allowed_template_ids = @allowed_template_ids::uuid[],
    mcp_server_ids = @mcp_server_ids::uuid[],
    external_auth_provider_ids = @external_auth_provider_ids::text[],
    template_parameter_rules = @template_parameter_rules,
    invokable_agent_ids = @invokable_agent_ids::uuid[],
    workspace_tools_enabled = @workspace_tools_enabled::boolean,
    model_config_id = sqlc.narg('model_config_id')::uuid,
    updated_at = now()
WHERE id = @id::uuid
RETURNING *;

-- name: DeleteChatProjectByID :exec
DELETE FROM chat_projects
WHERE id = @id::uuid;

-- name: CountChatProjectsByOwnerID :one
SELECT COUNT(*)::bigint
FROM chat_projects
WHERE owner_id = @owner_id::uuid;

-- name: RemoveChatProjectFromInvokableAgents :exec
-- Removes a deleted agent from every agent that was allowed to invoke it.
UPDATE chat_projects
SET
    invokable_agent_ids = array_remove(invokable_agent_ids, @project_id::uuid),
    updated_at = now()
WHERE @project_id::uuid = ANY (invokable_agent_ids);

-- name: UpdateChatProjectAgentUserID :one
-- Sets the agent user once; a project that already has one is unchanged.
UPDATE chat_projects
SET agent_user_id = @agent_user_id::uuid
WHERE id = @id::uuid
    AND agent_user_id IS NULL
RETURNING *;

-- name: GetChatProjectIDsWithoutAgentUser :many
SELECT id
FROM chat_projects
WHERE agent_user_id IS NULL
ORDER BY created_at, id;

-- name: UpdateChatProjectProfileByAgentUserID :one
-- Renames the agent an agent user acts as when the user's profile changes.
UPDATE chat_projects
SET
    name = @name::text,
    icon = @icon::text,
    updated_at = now()
WHERE agent_user_id = @agent_user_id::uuid
RETURNING *;

-- name: SyncChatProjectAgentUserProfiles :exec
-- Copies each agent's name and icon to its agent user, which shows the agent
-- wherever users are listed.
UPDATE users
SET
    name = chat_projects.name,
    avatar_url = chat_projects.icon,
    updated_at = now()
FROM chat_projects
WHERE chat_projects.agent_user_id = users.id
    AND (users.name <> chat_projects.name OR users.avatar_url <> chat_projects.icon);

-- name: GetChatProjectByAgentUserID :one
SELECT *
FROM chat_projects
WHERE agent_user_id = @agent_user_id::uuid;

-- name: GetChatProjectsByIDs :many
SELECT *
FROM chat_projects
WHERE id = ANY (@ids::uuid[])
ORDER BY lower(name), id;

-- name: GetChatProjectRuns :many
-- Returns root and delegated chats bound to a project, newest first.
SELECT *
FROM chats_expanded
WHERE project_id = @project_id::uuid
    AND archived = false
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('limit_opt')::int;
