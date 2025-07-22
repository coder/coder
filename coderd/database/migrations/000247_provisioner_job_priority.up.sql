-- Add priority column to provisioner_jobs table to support prioritizing human-initiated jobs over prebuilds
ALTER TABLE provisioner_jobs ADD COLUMN priority integer NOT NULL DEFAULT 0;

-- Create index for efficient priority-based ordering
CREATE INDEX idx_provisioner_jobs_priority_created_at ON provisioner_jobs (organization_id, started_at, priority DESC, created_at ASC) WHERE started_at IS NULL;

-- Update existing jobs to set priority based on whether they are prebuilds
-- Priority 1 = human-initiated jobs, Priority 0 = prebuilds
UPDATE provisioner_jobs 
SET priority = CASE 
    WHEN initiator_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0' THEN 0  -- PrebuildsSystemUserID
    ELSE 1  -- Human-initiated
END
WHERE started_at IS NULL;  -- Only update pending jobs
