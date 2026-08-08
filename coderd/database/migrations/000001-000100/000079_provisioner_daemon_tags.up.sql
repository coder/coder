ALTER TABLE provisioner_daemons ADD COLUMN tags jsonb NOT NULL DEFAULT '{}';

-- We must add the organization scope by default, otherwise pending jobs
-- could be provisioned on new daemons that don't match the tags.
ALTER TABLE provisioner_jobs ADD COLUMN tags jsonb NOT NULL DEFAULT '{"scope":"organization"}';
