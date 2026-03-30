-- Grant 'chat-access' to every user who has ever created a chat.
UPDATE users
SET rbac_roles = array_append(rbac_roles, 'chat-access')
WHERE id IN (SELECT DISTINCT owner_id FROM chats)
  AND NOT ('chat-access' = ANY(rbac_roles));
