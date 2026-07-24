ALTER TABLE organizations
    ADD COLUMN default_org_member_roles text[];

UPDATE organizations
SET default_org_member_roles = ARRAY['organization-workspace-access']::text[];

ALTER TABLE organizations
    ALTER COLUMN default_org_member_roles SET NOT NULL;

COMMENT ON COLUMN organizations.default_org_member_roles IS
    'Roles granted to every member of this organization at request time. '
    'The set is unioned into each member''s effective roles when '
    'GetAuthorizationUserRoles runs, so changes propagate to all members '
    'on the next request. Deployments can use this column to revoke '
    'capabilities that would otherwise be considered normal organization '
    'member permissions.';
