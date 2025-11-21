ALTER TABLE ONLY
	provisioner_job_timings
	ADD COLUMN stage_seq integer NOT NULL DEFAULT 0;

COMMENT ON COLUMN
	provisioner_job_timings.stage_seq IS
		'Distinguish repeated runs of the same stage within a single build job.';
