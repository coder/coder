ALTER TABLE ONLY users
	ADD COLUMN IF NOT EXISTS rbac_roles text[] DEFAULT '{}' NOT NULL;

-- All users are site members. So give them the standard role.
-- Also give them membership to the first org we retrieve. We should only have
-- 1 organization at this point in the product.
UPDATE
    users
SET
    rbac_roles = ARRAY ['member', 'organization-member:' || (SELECT id FROM organizations LIMIT 1)];

-- Give the first user created the admin role
UPDATE
	users
SET
	rbac_roles = rbac_roles || ARRAY ['admin']
WHERE
	id = (SELECT id FROM users ORDER BY created_at ASC LIMIT 1)
