-- The default was 'organization-member', but we imply that in the
-- 'GetAuthorizationUserRoles' query.
ALTER TABLE ONLY organization_members ALTER COLUMN roles SET DEFAULT '{}';

-- No one should be using organization roles yet. If they are, the names in the
-- database are now incorrect. Just remove them all.
UPDATE organization_members SET roles = '{}';
