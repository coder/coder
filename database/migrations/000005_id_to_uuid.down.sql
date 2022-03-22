DROP INDEX idx_organization_members_id_uuid;
ALTER TABLE organization_members DROP COLUMN id_uuid;

DROP INDEX idx_organization_members_id_uuid;
ALTER TABLE organizations DROP COLUMN user_id_uuid;
ALTER TABLE organizations DROP COLUMN organization_id_uuid;

DROP INDEX idx_users_id_uuid;
ALTER TABLE users DROP COLUMN id_uuid;
