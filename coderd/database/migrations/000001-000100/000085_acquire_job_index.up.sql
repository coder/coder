CREATE INDEX provisioner_jobs_started_at_idx ON provisioner_jobs USING btree (started_at)
    WHERE started_at IS NULL;
