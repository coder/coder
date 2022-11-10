ALTER TABLE provisioner_daemons ADD COLUMN tags jsonb NOT NULL DEFAULT '{}';
ALTER TABLE provisioner_jobs ADD COLUMN tags jsonb NOT NULL DEFAULT '{}';
