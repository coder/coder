ALTER TABLE network_policy_targets DROP CONSTRAINT network_policy_targets_state_run_fkey,
    DROP COLUMN state_run_id, DROP COLUMN destroyed_at;
ALTER TABLE network_policy_runs DROP CONSTRAINT network_policy_runs_state_provenance_key;
-- Retained destroy history cannot be represented by the older schema.
ALTER TABLE network_policy_runs DROP CONSTRAINT network_policy_runs_operation_check;
ALTER TABLE network_policy_runs ADD CONSTRAINT network_policy_runs_operation_check CHECK (operation IN ('plan', 'apply')) NOT VALID;
ALTER TABLE network_policies DROP CONSTRAINT network_policies_deletion_check,
    DROP COLUMN deleted_at, DROP COLUMN deletion_requested_at;
