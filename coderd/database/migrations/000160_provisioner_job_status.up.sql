CREATE TYPE provisioner_job_status AS ENUM ('pending', 'running', 'succeeded', 'canceling', 'canceled', 'failed', 'unknown');
COMMENT ON TYPE provisioner_job_status IS 'Computed status of a provisioner job. Jobs could be stuck in a hung state, these states do not guarantee any transition to another state.';

ALTER TABLE provisioner_jobs ADD COLUMN
    job_status provisioner_job_status NOT NULL GENERATED ALWAYS AS (
        CASE
            -- Completed means it is not in an "-ing" state
            WHEN completed_at IS NOT NULL THEN
                CASE
                    -- The order of these checks are important.
                    -- Check the error first, then cancelled, then completed.
                    WHEN error != '' THEN 'failed'::provisioner_job_status
                    WHEN canceled_at IS NOT NULL THEN 'canceled'::provisioner_job_status
                    ELSE 'succeeded'::provisioner_job_status
                END
            -- Not completed means it is in some "-ing" state
            ELSE
                CASE
                    -- This should never happen because all errors set
                    -- should also set a completed_at timestamp.
                    -- But if there is an error, we should always return
                    -- a failed state.
                    WHEN error != '' THEN 'failed'::provisioner_job_status
                    WHEN canceled_at IS NOT NULL THEN 'canceling'::provisioner_job_status
                    -- Not done and not started means it is pending
                    WHEN started_at IS NULL THEN 'pending'::provisioner_job_status
                    ELSE 'running'::provisioner_job_status
                END
        END
        -- Stored so we do not have to recompute it every time.
        ) STORED;


COMMENT ON COLUMN provisioner_jobs.job_status IS 'Computed column to track the status of the job.';
