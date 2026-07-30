INSERT INTO provisioner_keys (id, created_at, organization_id, name, hashed_secret, tags) VALUES ('00000000-0000-0000-0000-000000000001'::uuid, NOW(), (SELECT id FROM organizations WHERE is_default = true), 'built-in', ''::bytea, '{}');
INSERT INTO provisioner_keys (id, created_at, organization_id, name, hashed_secret, tags) VALUES ('00000000-0000-0000-0000-000000000002'::uuid, NOW(), (SELECT id FROM organizations WHERE is_default = true), 'user-auth', ''::bytea, '{}');
INSERT INTO provisioner_keys (id, created_at, organization_id, name, hashed_secret, tags) VALUES ('00000000-0000-0000-0000-000000000003'::uuid, NOW(), (SELECT id FROM organizations WHERE is_default = true), 'psk', ''::bytea, '{}');

ALTER TABLE provisioner_daemons ADD COLUMN key_id UUID REFERENCES provisioner_keys(id) ON DELETE CASCADE DEFAULT '00000000-0000-0000-0000-000000000001'::uuid NOT NULL;
ALTER TABLE provisioner_daemons ALTER COLUMN key_id DROP DEFAULT;
