ALTER TABLE provisioner_job_logs DROP COLUMN id;

ALTER TABLE provisioner_job_logs ADD COLUMN id uuid NOT NULL DEFAULT gen_random_uuid();
