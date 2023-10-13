BEGIN;

ALTER TABLE provisioner_jobs DROP COLUMN job_status;
DROP TYPE provisioner_job_status;

COMMIT;
