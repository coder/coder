-- name: InsertChatProject :one
INSERT INTO chat_projects (
    id, organization_id, owner_id, name, description, icon,
    limit_templates, default_template_id, default_template_version_preset_id, default_parameters
)
VALUES (
    COALESCE(sqlc.narg('id')::uuid, gen_random_uuid()),
    @organization_id::uuid,
    @owner_id::uuid,
    @name::text,
    @description::text,
    @icon::text,
    @limit_templates::boolean,
    sqlc.narg('default_template_id')::uuid,
    sqlc.narg('default_template_version_preset_id')::uuid,
    COALESCE(sqlc.narg('default_parameters')::jsonb, '{}'::jsonb)
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

-- name: GetChatProjectByIDForShare :one
-- FOR SHARE conflicts with sharing-list updates, which lock the row FOR
-- UPDATE, so a chat attached under this lock cannot miss a removal.
SELECT *
FROM chat_projects
WHERE id = @id::uuid
FOR SHARE;

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

-- name: IsChatProjectUsableByUserID :one
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

-- name: RemoveChatsFromUnusableChatProjects :many
-- Moves chats out of projects their owners can no longer use. Callers run it
-- after any change that can remove project access, narrowed by whichever of
-- project, owner, and organization the change affects. The access check must
-- match IsChatProjectUsableByUserID (migration 000644 copies it for a one-time
-- backfill). Only root chats have a project_id, and
-- updated_at is not touched, matching UpdateChatProjectIDByID. Rows are locked
-- in id order so concurrent removals cannot deadlock each other.
WITH unusable AS (
    SELECT chats.id
    FROM chats
    JOIN chat_projects ON chat_projects.id = chats.project_id
    WHERE
        (sqlc.narg('project_id')::uuid IS NULL OR chats.project_id = sqlc.narg('project_id')::uuid)
        AND (sqlc.narg('owner_id')::uuid IS NULL OR chats.owner_id = sqlc.narg('owner_id')::uuid)
        AND (sqlc.narg('organization_id')::uuid IS NULL OR chats.organization_id = sqlc.narg('organization_id')::uuid)
        AND chat_projects.owner_id != chats.owner_id
        -- A missing ACL entry makes the check NULL, which counts as no access.
        AND NOT COALESCE((
            EXISTS (
                SELECT 1
                FROM organization_members
                WHERE organization_members.user_id = chats.owner_id
                    AND organization_members.organization_id = chat_projects.organization_id
            )
            AND (
                chat_projects.user_acl -> chats.owner_id::text -> 'permissions' ?| ARRAY['read', '*']
                OR chat_projects.group_acl -> chat_projects.organization_id::text -> 'permissions' ?| ARRAY['read', '*']
                OR EXISTS (
                    SELECT 1
                    FROM group_members
                    WHERE group_members.user_id = chats.owner_id
                        AND chat_projects.group_acl -> group_members.group_id::text -> 'permissions' ?| ARRAY['read', '*']
                )
            )
        ), false)
    ORDER BY chats.id
    FOR UPDATE OF chats
)
UPDATE chats
SET project_id = NULL
FROM unusable
WHERE chats.id = unusable.id
RETURNING chats.id;

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
    -- The template settings change only when the request sets them, so a
    -- concurrent update that omits them cannot restore a stale snapshot.
    limit_templates = COALESCE(sqlc.narg('limit_templates')::boolean, limit_templates),
    default_template_id = CASE WHEN @update_default_template::boolean THEN sqlc.narg('default_template_id')::uuid ELSE default_template_id END,
    default_template_version_preset_id = CASE WHEN @update_default_template::boolean THEN sqlc.narg('default_template_version_preset_id')::uuid ELSE default_template_version_preset_id END,
    default_parameters = CASE WHEN @update_default_template::boolean THEN COALESCE(sqlc.narg('default_parameters')::jsonb, '{}'::jsonb) ELSE default_parameters END,
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

-- name: InsertChatProjectTemplates :many
-- Attaches templates to a project and returns the new attachments. Templates
-- that are already attached are skipped, so their rows keep their original
-- created_by. Templates outside the project's organization are skipped too,
-- so an attachment never crosses organizations.
INSERT INTO chat_project_templates (project_id, template_id, created_by)
SELECT chat_projects.id, templates.id, @created_by::uuid
FROM chat_projects
JOIN templates ON templates.organization_id = chat_projects.organization_id
WHERE chat_projects.id = @project_id::uuid
    AND templates.id = ANY(@template_ids::uuid[])
ON CONFLICT (project_id, template_id) DO NOTHING
RETURNING *;

-- name: DeleteChatProjectTemplate :one
-- Detaches a template from a project. Detaching the default template also
-- clears the project's defaults, which only apply to an attached template.
-- Nothing changes when the template is not attached.
WITH cleared AS (
    UPDATE chat_projects
    SET
        default_template_id = NULL,
        default_template_version_preset_id = NULL,
        default_parameters = '{}'::jsonb
    WHERE id = @project_id::uuid
        AND default_template_id = @template_id::uuid
        AND EXISTS (
            SELECT 1
            FROM chat_project_templates
            WHERE project_id = @project_id::uuid
                AND template_id = @template_id::uuid
        )
)
DELETE FROM chat_project_templates
WHERE project_id = @project_id::uuid
    AND template_id = @template_id::uuid
RETURNING *;

-- name: GetChatProjectTemplatesByProjectIDs :many
-- Lists the templates attached to the given projects, oldest attachment
-- first. Deleted, deprecated, and agent-disabled templates are left out
-- because no chat can use them, whoever asks. The rows are not filtered by
-- the caller's access to each template, so callers must check that before
-- exposing a template.
SELECT
    sqlc.embed(chat_project_templates),
    sqlc.embed(chat_projects)
FROM chat_project_templates
JOIN chat_projects ON chat_projects.id = chat_project_templates.project_id
JOIN templates ON templates.id = chat_project_templates.template_id
WHERE chat_project_templates.project_id = ANY(@project_ids::uuid[])
    AND NOT templates.deleted
    AND templates.deprecated = ''
    AND templates.agents_allowed
ORDER BY
    chat_project_templates.project_id,
    chat_project_templates.created_at,
    chat_project_templates.template_id;
