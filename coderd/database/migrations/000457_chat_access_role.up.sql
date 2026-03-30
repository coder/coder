-- Grant 'agents-access' to every user who has ever created a chat.
UPDATE users
SET rbac_roles = array_append(rbac_roles, 'agents-access')
WHERE id IN (SELECT DISTINCT owner_id FROM chats)
  AND NOT ('agents-access' = ANY(rbac_roles));
