ALTER TABLE network_policies
    ADD COLUMN deletion_requested_at timestamptz,
    ADD COLUMN deleted_at timestamptz,
    ADD CONSTRAINT network_policies_deletion_check CHECK (deleted_at IS NULL OR deletion_requested_at IS NOT NULL);

ALTER TABLE network_policy_runs DROP CONSTRAINT network_policy_runs_operation_check;
ALTER TABLE network_policy_runs ADD CONSTRAINT network_policy_runs_operation_check CHECK (operation IN ('plan', 'apply', 'destroy'));
ALTER TABLE network_policy_runs ADD CONSTRAINT network_policy_runs_state_provenance_key UNIQUE (id, target_id, policy_id, organization_id);

ALTER TABLE network_policy_targets
    ADD COLUMN destroyed_at timestamptz,
    ADD COLUMN state_run_id uuid,
    ADD CONSTRAINT network_policy_targets_state_run_fkey FOREIGN KEY (state_run_id, id, policy_id, organization_id)
        REFERENCES network_policy_runs(id, target_id, policy_id, organization_id);

-- Do not infer state provenance from published or applied revision pointers.
-- The server validates retained history before adopting legacy state, and never
-- releases reservations whose infrastructure outcome is unknown.
