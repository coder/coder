INSERT INTO provisioner_keys (id, created_at, organization_id, name, hashed_secret, tags) VALUES ('11111111-1111-1111-1111-111111111111'::uuid, NOW(), (SELECT id FROM organizations WHERE is_default = true), 'built-in', ''::bytea, '{}');
INSERT INTO provisioner_keys (id, created_at, organization_id, name, hashed_secret, tags) VALUES ('22222222-2222-2222-2222-222222222222'::uuid, NOW(), (SELECT id FROM organizations WHERE is_default = true), 'user-auth', ''::bytea, '{}');
INSERT INTO provisioner_keys (id, created_at, organization_id, name, hashed_secret, tags) VALUES ('33333333-3333-3333-3333-333333333333'::uuid, NOW(), (SELECT id FROM organizations WHERE is_default = true), 'psk', ''::bytea, '{}');

ALTER TABLE provisioner_daemons ADD COLUMN key_id UUID REFERENCES provisioner_keys(id) ON DELETE CASCADE DEFAULT '11111111-1111-1111-1111-111111111111'::uuid NOT NULL;
