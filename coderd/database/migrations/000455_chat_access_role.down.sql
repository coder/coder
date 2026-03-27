-- Remove 'chat-access' from all users.
UPDATE users
SET rbac_roles = array_remove(rbac_roles, 'chat-access');
