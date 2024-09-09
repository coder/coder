INSERT INTO provisioner_keys (id, created_at, name, public_key, organization_id) VALUES (gen_random_uuid(), NOW(), 'built-in', '', (SELECT id FROM organizations WHERE is_default = true));
INSERT INTO provisioner_keys (id, created_at, name, public_key, organization_id) VALUES (gen_random_uuid(), NOW(), 'psk', '', (SELECT id FROM organizations WHERE is_default = true));

ALTER TABLE provisioner_daemons ADD COLUMN key_id UUID NOT NULL REFERENCES provisioner_keys(id) DEFAULT (SELECT id FROM provisioner_keys WHERE name = 'built-in');
