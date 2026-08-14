-- The exact set of users who held 'agents-access' cannot be
-- reconstructed, so restore it conservatively: binaries that predate the
-- role removal require the role for chat access, and without a restore
-- every organization member would lose access to their existing chats on
-- rollback. Service accounts never held chat access and are excluded.
UPDATE organization_members om
SET roles = array_append(om.roles, 'agents-access')
FROM users u
WHERE u.id = om.user_id
  AND NOT u.is_service_account
  AND NOT ('agents-access' = ANY(om.roles));
