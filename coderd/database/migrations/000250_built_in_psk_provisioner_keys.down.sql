ALTER TABLE provisioner_daemons DROP COLUMN key_id;

DELETE FROM provisioner_keys WHERE name = 'built-in';
DELETE FROM provisioner_keys WHERE name = 'psk';
DELETE FROM provisioner_keys WHERE name = 'user-auth';
