-- Custom roles named agents-access could predate the built-in role. The
-- built-in would shadow them and make them undeletable.
DELETE FROM custom_roles WHERE name = 'agents-access';

UPDATE organizations
SET default_org_member_roles = array_append(default_org_member_roles, 'agents-access')
WHERE NOT ('agents-access' = ANY(default_org_member_roles));
