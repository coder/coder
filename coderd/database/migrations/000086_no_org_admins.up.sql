UPDATE 
  organization_members
SET 
  roles = ARRAY [] :: text[]
WHERE 
  'organization-admin:'||organization_id = ANY(roles);
