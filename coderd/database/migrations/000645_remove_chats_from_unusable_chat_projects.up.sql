-- Moves chats out of projects their owners can no longer use. Chats became
-- stranded this way before access removals started moving them. The access
-- check matches RemoveChatsFromUnusableChatProjects.
UPDATE chats
SET project_id = NULL
FROM chat_projects
WHERE chat_projects.id = chats.project_id
    AND chat_projects.owner_id != chats.owner_id
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
    ), false);
