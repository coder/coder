-- Remove the priority-based index
DROP INDEX IF EXISTS idx_provisioner_jobs_priority_created_at;

-- Remove the priority column
ALTER TABLE provisioner_jobs DROP COLUMN IF EXISTS priority;
