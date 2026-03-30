-- Remove 'chat-access' from all users who have it.
UPDATE users
SET rbac_roles = array_remove(rbac_roles, 'chat-access')
WHERE 'chat-access' = ANY(rbac_roles);
