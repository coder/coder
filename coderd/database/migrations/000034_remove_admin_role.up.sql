UPDATE
    users
SET
    -- Replace the role 'admin' with the role 'owner'
    rbac_roles = array_replace(rbac_roles, 'admin', 'owner')
WHERE
    -- Update the first user with the role 'admin'. This should be the first
    -- user ever, but if that user was demoted from an admin, then choose
    -- the next best user.
    id = (SELECT id FROM users WHERE 'admin' = ANY(rbac_roles) ORDER BY created_at ASC LIMIT 1);


UPDATE
    users
SET
    -- Replace 'admin' role with 'template-admin' and 'user-admin'
    rbac_roles = array_cat(array_remove(rbac_roles, 'admin'), ARRAY ['template-admin', 'user-admin'])
WHERE
    -- Only on existing admins
    'admin' = ANY(rbac_roles);
