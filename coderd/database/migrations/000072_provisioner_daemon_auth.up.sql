ALTER TABLE provisioner_daemons ADD COLUMN tags jsonb;

ALTER TABLE template_versions ADD COLUMN provisioner_tags jsonb;
