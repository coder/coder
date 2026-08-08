CREATE INDEX provisioner_job_logs_id_job_id_idx ON provisioner_job_logs USING btree (job_id, id ASC);
