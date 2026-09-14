-- Previous custom roles are now invalid, as the json changed. Since this is an
-- experimental feature, there is no point in trying to save the perms.
-- This does not elevate any permissions, so it is not a security issue.
UPDATE custom_roles SET org_permissions = '[]';
